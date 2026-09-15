package scopeapp

import (
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/platform/metrics"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	scopeport "github.com/gsoultan/anubis/internal/scope/port"
)

// The embedded nil interfaces satisfy the rest of each port; a method these
// tests do not exercise panics rather than quietly returning a zero value.

type dueSyncRepo struct {
	scopeport.ScopeSyncRepository
	due         []scopedomain.SyncSourceRecord
	rescheduled []string
	failures    []string
	applyErr    error
	applied     int
}

func (r *dueSyncRepo) RecordSyncFailure(_ context.Context, _, reason string) error {
	r.failures = append(r.failures, reason)
	return nil
}

func (r *dueSyncRepo) DueSyncSources(context.Context, time.Time, int32) ([]scopedomain.SyncSourceRecord, error) {
	return r.due, nil
}

func (r *dueSyncRepo) ScheduleNextSyncSource(_ context.Context, id string) error {
	r.rescheduled = append(r.rescheduled, id)
	return nil
}

func (r *dueSyncRepo) ScopeSyncApply(context.Context, string, []byte, bool) (string, error) {
	if r.applyErr != nil {
		return "", r.applyErr
	}
	r.applied++
	return `{"added":1}`, nil
}

type rootNodes struct {
	scopeport.ScopeNodeRepository
}

func (rootNodes) EnsureAxisRoot(context.Context, string, string) (string, error) {
	return "root-id", nil
}

type stubFetcher struct {
	rows []scopedomain.SyncFeedRow
	err  error
}

func (f stubFetcher) Fetch(context.Context, scopedomain.SyncSourceRecord) ([]scopedomain.SyncFeedRow, error) {
	return f.rows, f.err
}

type recordingAuditor struct{ events []auditdomain.AuditEvent }

func (a *recordingAuditor) Emit(_ context.Context, ev auditdomain.AuditEvent) {
	a.events = append(a.events, ev)
}

func schedulerUnderTest(repo *dueSyncRepo, f scopeport.ScopeFeedFetcher) (*scopeAdminInteractor, *recordingAuditor) {
	audit := &recordingAuditor{}
	return &scopeAdminInteractor{
		sync: repo, nodes: rootNodes{}, fetcher: f, audit: audit,
		logger: slog.New(slog.DiscardHandler),
	}, audit
}

func oneDueSource() []scopedomain.SyncSourceRecord {
	return []scopedomain.SyncSourceRecord{{
		ID: "src-1", TenantID: "tenant-1", Axis: "org", Kind: "http",
		Status: "active", Config: []byte(`{"url":"https://erp.example/units"}`),
		IntervalSeconds: 300,
	}}
}

// A feed that is down must still move its source on. Without this the next
// tick finds it due again a minute later, forever, and a scheduled sync
// becomes a hot loop against somebody else's server.
func TestRunDueReschedulesAfterAFailedFetch(t *testing.T) {
	repo := &dueSyncRepo{due: oneDueSource()}
	u, audit := schedulerUnderTest(repo, stubFetcher{err: errors.New("connection refused")})

	ran, err := u.RunDue(context.Background(), time.Now(), 0)
	if err != nil {
		t.Fatalf("RunDue returned %v; one bad feed must not fail the batch", err)
	}
	if ran != 1 {
		t.Errorf("ran = %d, want 1", ran)
	}
	if len(repo.rescheduled) != 1 || repo.rescheduled[0] != "src-1" {
		t.Errorf("rescheduled = %v, want [src-1] — a failed run still moves on", repo.rescheduled)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "sync.fetch_failed" {
		t.Fatalf("events = %+v, want one sync.fetch_failed", audit.events)
	}
	// scope_sync_apply records everything that reaches it, and a fetch that
	// fails never does. Without a row here the Source pane shows a schedule
	// and an empty history while nothing has synced for days.
	if len(repo.failures) != 1 {
		t.Fatalf("failures = %v, want one recorded run", repo.failures)
	}
	if !strings.Contains(repo.failures[0], "connection refused") {
		t.Errorf("recorded reason = %q, want it to name the cause", repo.failures[0])
	}
}

// A run nobody asked for must not wear somebody's identity. The scheduler has
// no principal, so the entry has to say system and carry no actor id.
func TestRunDueAuditsAsSystem(t *testing.T) {
	repo := &dueSyncRepo{due: oneDueSource()}
	u, audit := schedulerUnderTest(repo, stubFetcher{
		rows: []scopedomain.SyncFeedRow{{Ref: "jkt", Name: "Jakarta"}},
	})

	if _, err := u.RunDue(context.Background(), time.Now(), 0); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if repo.applied != 1 {
		t.Fatalf("applied = %d, want 1", repo.applied)
	}
	if len(audit.events) != 1 {
		t.Fatalf("events = %+v, want one", audit.events)
	}
	ev := audit.events[0]
	if ev.Action != "sync.run" {
		t.Errorf("action = %q, want sync.run", ev.Action)
	}
	if ev.ActorKind != "system" {
		t.Errorf("actor_kind = %q, want system", ev.ActorKind)
	}
	if ev.ActorID != "" {
		t.Errorf("actor_id = %q, want empty — a timer has no operator", ev.ActorID)
	}
	if ev.TenantID != "tenant-1" {
		t.Errorf("tenant = %q, want tenant-1 — taken from the record, not an ambient principal", ev.TenantID)
	}
}

// A feed returning nothing means "every node you have is gone", and the
// reconciler would archive the whole axis. It must refuse instead — and still
// reschedule, or the refusal becomes the hot loop.
func TestRunDueRefusesAnEmptyFeed(t *testing.T) {
	repo := &dueSyncRepo{due: oneDueSource()}
	u, _ := schedulerUnderTest(repo, stubFetcher{rows: nil})

	if _, err := u.RunDue(context.Background(), time.Now(), 0); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if repo.applied != 0 {
		t.Errorf("applied = %d, want 0 — an empty feed must archive nothing", repo.applied)
	}
	if len(repo.rescheduled) != 1 {
		t.Errorf("rescheduled = %v, want one entry", repo.rescheduled)
	}
	// The refusal is the whole point, so it has to be visible afterwards.
	if len(repo.failures) != 1 || !strings.Contains(repo.failures[0], "zero rows") {
		t.Errorf("failures = %v, want the zero-row refusal recorded", repo.failures)
	}
}

// The reconciler writes its own row, so a run that reaches it must not get a
// second one from the app tier.
func TestRunDueRecordsNoFailureWhenTheRunSucceeds(t *testing.T) {
	repo := &dueSyncRepo{due: oneDueSource()}
	u, _ := schedulerUnderTest(repo, stubFetcher{
		rows: []scopedomain.SyncFeedRow{{Ref: "jkt", Name: "Jakarta"}},
	})

	if _, err := u.RunDue(context.Background(), time.Now(), 0); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if len(repo.failures) != 0 {
		t.Errorf("failures = %v, want none — scope_sync_apply records its own run", repo.failures)
	}
}

// RunDue returns nil when a source fails, so anubis_job_runs_total stays "ok"
// however many feeds are broken. The counter is the only thing that knows, so
// it has to move — otherwise a structure goes stale for a week with every
// operational signal green.
//
// Asserted against the real exposition rather than a reader added to the
// metrics package for the benefit of a test: what Prometheus sees is the thing
// that matters, and a counter nothing scrapes is not instrumentation.
func TestRunDueCountsOutcomes(t *testing.T) {
	scrape := func() string {
		rec := httptest.NewRecorder()
		metrics.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
		return rec.Body.String()
	}
	// Axis codes unique to this test, so a count is exact rather than "more
	// than it was" and no other test can move it.
	failing := oneDueSource()
	failing[0].Axis = "runduetest_failing"
	working := oneDueSource()
	working[0].Axis = "runduetest_working"

	repo := &dueSyncRepo{due: failing}
	u, _ := schedulerUnderTest(repo, stubFetcher{err: errors.New("connection refused")})
	if _, err := u.RunDue(context.Background(), time.Now(), 0); err != nil {
		t.Fatalf("RunDue: %v", err)
	}

	repo2 := &dueSyncRepo{due: working}
	u2, _ := schedulerUnderTest(repo2, stubFetcher{
		rows: []scopedomain.SyncFeedRow{{Ref: "jkt", Name: "Jakarta"}},
	})
	if _, err := u2.RunDue(context.Background(), time.Now(), 0); err != nil {
		t.Fatalf("RunDue: %v", err)
	}

	body := scrape()
	for _, want := range []string{
		`anubis_scope_sync_runs_total{axis="runduetest_failing",result="failed"} 1`,
		`anubis_scope_sync_runs_total{axis="runduetest_working",result="ok"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("exposition missing %q", want)
		}
	}
}
