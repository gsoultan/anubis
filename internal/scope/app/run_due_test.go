package scopeapp

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	scopeport "github.com/gsoultan/anubis/internal/scope/port"
)

// The embedded nil interfaces satisfy the rest of each port; a method these
// tests do not exercise panics rather than quietly returning a zero value.

type dueSyncRepo struct {
	scopeport.ScopeSyncRepository
	due         []scopedomain.SyncSourceRecord
	rescheduled []string
	applyErr    error
	applied     int
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
}
