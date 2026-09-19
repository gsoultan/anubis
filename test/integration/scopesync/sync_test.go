//go:build integration

// Package scopesync covers the scope context's scheduling, which is the part
// of it that moved to storm with logic rather than just columns.
//
// The reschedule CASE is the subtle one: changing a source's interval moves it
// to due-now, leaving the interval alone leaves the existing due time exactly
// where it was. Saving an unrelated edit must not quietly restart the clock on
// a feed that was about to run — and the comparison is against the column's
// CURRENT value, which only the database has.
package scopesync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gsoultan/anubis/internal/platform/database"
	scopepg "github.com/gsoultan/anubis/internal/scope/adapter/postgres"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	pool   *pgxpool.Pool
	tenant string
	axis   string
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("ANUBIS_DB_URL")
	if dsn == "" {
		os.Exit(0)
	}
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		panic(err)
	}
	pool = p
	ctx := context.Background()
	n := time.Now().UnixNano()
	slug := fmt.Sprintf("zzsync%d", n%1_000_000_000)
	if err := p.QueryRow(ctx,
		`INSERT INTO tenants (slug, name) VALUES ($1, 'Scope sync probe') RETURNING id`,
		slug).Scan(&tenant); err != nil {
		panic("create probe tenant: " + err.Error())
	}
	// Its own axis, so nothing here touches a real one.
	axis = fmt.Sprintf("zz%d", n%100_000_000)
	if _, err := p.Exec(ctx,
		`INSERT INTO scope_axes (code, display_name) VALUES ($1, 'probe')`, axis); err != nil {
		panic("create probe axis: " + err.Error())
	}
	code := m.Run()
	_, _ = p.Exec(ctx, `DELETE FROM scope_sync_sources WHERE tenant_id = $1`, tenant)
	_, _ = p.Exec(ctx, `DELETE FROM scope_nodes WHERE tenant_id = $1`, tenant)
	_, _ = p.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenant)
	_, _ = p.Exec(ctx, `DELETE FROM scope_axes WHERE code = $1`, axis)
	p.Close()
	os.Exit(code)
}

func repo(t *testing.T) *scopepg.Repository {
	t.Helper()
	if pool == nil {
		t.Skip("ANUBIS_DB_URL not set")
	}
	return scopepg.New(database.New(pool))
}

// mkSourceOn creates a source on a given axis. A source is unique per
// (tenant, axis), so a test that needs two needs two axes.
func mkSourceOn(t *testing.T, r *scopepg.Repository, ax string, interval int32) string {
	t.Helper()
	cfg, _ := json.Marshal(map[string]string{"url": "https://example.test/feed"})
	id, err := r.CreateSyncSource(context.Background(), tenant, scopedomain.SyncSourceRecord{
		Axis: ax, Kind: "http", Config: cfg, IntervalSeconds: interval,
	})
	if err != nil {
		t.Fatalf("create sync source: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM scope_sync_sources WHERE id = $1`, id)
	})
	return id
}

func mkSource(t *testing.T, r *scopepg.Repository, interval int32) string {
	t.Helper()
	return mkSourceOn(t, r, axis, interval)
}

// mkAxis makes a throwaway axis so a test can hold more than one source.
func mkAxis(t *testing.T) string {
	t.Helper()
	code := fmt.Sprintf("zz%d", time.Now().UnixNano()%100_000_000)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO scope_axes (code, display_name) VALUES ($1, 'probe')`, code); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM scope_axes WHERE code = $1`, code)
	})
	return code
}

// A source created WITH an interval is due immediately: the operator who just
// pointed Anubis at an ERP expects the tree to fill, not to sit empty until
// the first interval elapses. Created without one, it is manual and never due.
func TestCreateSchedulesOnlyWhenIntervalled(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	scheduled := mkSource(t, r, 600)
	got, err := r.SyncSource(ctx, tenant, scheduled)
	if err != nil {
		t.Fatal(err)
	}
	if got.NextRunAt == nil {
		t.Fatal("a source created with an interval is not due")
	}
	if time.Until(*got.NextRunAt) > time.Minute {
		t.Fatalf("due at %s, which is not now", got.NextRunAt)
	}

	manual := mkSourceOn(t, r, mkAxis(t), 0)
	got, err = r.SyncSource(ctx, tenant, manual)
	if err != nil {
		t.Fatal(err)
	}
	if got.NextRunAt != nil {
		t.Fatalf("a manual source is scheduled for %s", got.NextRunAt)
	}
}

// The CASE. Saving an edit that does not touch the interval must leave the due
// time alone; changing the interval moves it to now; zeroing it unschedules.
func TestRescheduleOnlyWhenTheIntervalChanges(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id := mkSource(t, r, 600)

	// Push the due time into the future so "unchanged" is observable.
	future := time.Now().Add(5 * time.Hour).UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx,
		`UPDATE scope_sync_sources SET next_run_at = $2 WHERE id = $1`, id, future); err != nil {
		t.Fatal(err)
	}

	cfg, _ := json.Marshal(map[string]string{"url": "https://example.test/changed"})
	// Same interval, different config: the clock must not restart.
	if err := r.UpdateSyncSource(ctx, tenant, scopedomain.SyncSourceRecord{
		ID: id, Config: cfg, Status: "active", IntervalSeconds: 600,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := r.SyncSource(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.NextRunAt == nil || !got.NextRunAt.UTC().Truncate(time.Second).Equal(future) {
		t.Fatalf("an unrelated edit moved the due time to %v, want %v", got.NextRunAt, future)
	}

	// A DIFFERENT interval reschedules from now.
	if err := r.UpdateSyncSource(ctx, tenant, scopedomain.SyncSourceRecord{
		ID: id, Config: cfg, Status: "active", IntervalSeconds: 900,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = r.SyncSource(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.NextRunAt == nil || time.Until(*got.NextRunAt) > time.Minute {
		t.Fatalf("changing the interval did not reschedule: %v", got.NextRunAt)
	}

	// Zero unschedules entirely.
	if err := r.SetSyncSchedule(ctx, tenant, id, 0); err != nil {
		t.Fatal(err)
	}
	got, err = r.SyncSource(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.NextRunAt != nil {
		t.Fatalf("a zero interval left the source due at %s", got.NextRunAt)
	}
}

// DueSyncSources is the scheduler's only read: active, scheduled, and due.
// Each of those three has to exclude, or a disabled feed keeps being fetched.
func TestDueSyncSourcesSelectsAndExcludes(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	due := mkSource(t, r, 600)

	found := func() bool {
		t.Helper()
		rows, err := r.DueSyncSources(ctx, time.Now(), 200)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range rows {
			if s.ID == due {
				return true
			}
		}
		return false
	}

	if !found() {
		t.Fatal("a source due now was not returned")
	}

	// Not due yet.
	if _, err := pool.Exec(ctx,
		`UPDATE scope_sync_sources SET next_run_at = now() + interval '1 hour' WHERE id = $1`, due); err != nil {
		t.Fatal(err)
	}
	if found() {
		t.Fatal("a source due in an hour was returned as due now")
	}

	// Due, but disabled.
	if _, err := pool.Exec(ctx,
		`UPDATE scope_sync_sources SET next_run_at = now(), status = 'disabled' WHERE id = $1`, due); err != nil {
		t.Fatal(err)
	}
	if found() {
		t.Fatal("a disabled source was returned as due")
	}

	// Due and active, but unscheduled.
	if _, err := pool.Exec(ctx,
		`UPDATE scope_sync_sources SET status = 'active', next_run_at = NULL WHERE id = $1`, due); err != nil {
		t.Fatal(err)
	}
	if found() {
		t.Fatal("a manual source was returned as due")
	}
}

// ScheduleNextSyncSource moves a source on whether or not the run worked, and
// moves next_run_at ONLY: last_run_at means "last succeeded" in this schema,
// and stamping it here would make the console report a time at which the sync
// had in fact failed.
func TestScheduleNextMovesOnlyTheDueTime(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id := mkSource(t, r, 600)

	before, err := r.SyncSource(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	if before.LastRunAt != nil {
		t.Fatal("a fresh source has already run")
	}

	if err := r.ScheduleNextSyncSource(ctx, id); err != nil {
		t.Fatal(err)
	}
	after, err := r.SyncSource(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.LastRunAt != nil {
		t.Fatalf("scheduling stamped last_run_at (%s) — that column means last SUCCEEDED", after.LastRunAt)
	}
	if after.NextRunAt == nil || !after.NextRunAt.After(time.Now().Add(9*time.Minute)) {
		t.Fatalf("next run is %v, want ~10 minutes out", after.NextRunAt)
	}
}

// RecordSyncFailure writes the run row for an attempt that never reached
// scope_sync_apply — a feed that cannot be fetched never gets there, and left
// one operator looking at an empty history while nothing had synced for days.
func TestRecordSyncFailureIsVisibleInTheHistory(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id := mkSource(t, r, 0)

	if err := r.RecordSyncFailure(ctx, id, "dial tcp: connection refused"); err != nil {
		t.Fatal(err)
	}
	runs, err := r.ListSyncRuns(ctx, tenant, id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("history has %d runs, want 1", len(runs))
	}
	if runs[0].Status != "failed" || runs[0].FinishedAt == nil {
		t.Fatalf("run is %+v, want a finished failure", runs[0])
	}
	var report struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(runs[0].Report, &report); err != nil {
		t.Fatalf("report is not the reconciler's shape: %v", err)
	}
	if report.Error != "dial tcp: connection refused" {
		t.Fatalf("report error is %q", report.Error)
	}

	// The history is scoped through the source's tenant; another tenant's id
	// must not read it.
	other, err := r.ListSyncRuns(ctx, "00000000-0000-0000-0000-000000000000", id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Fatalf("another tenant read %d runs of this feed", len(other))
	}
}
