package authzcatalog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/authz/domain/catalogsync"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

// --- fakes ------------------------------------------------------------------

type fakeRepo struct {
	sources     []catalogsync.Source
	runs        []catalogsync.Run
	lastDigest  string
	rescheduled int
	finished    []string // "status|digest|error", in order
	deleted     []string
	startErr    error
}

func (f *fakeRepo) ListSources(context.Context, string) ([]catalogsync.Source, error) {
	return f.sources, nil
}
func (f *fakeRepo) Source(_ context.Context, _, id string) (*catalogsync.Source, error) {
	for i := range f.sources {
		if f.sources[i].ID == id {
			return &f.sources[i], nil
		}
	}
	return nil, apperr.ErrNotFound
}
func (f *fakeRepo) DueSources(context.Context, time.Time, int32) ([]catalogsync.Source, error) {
	return f.sources, nil
}
func (f *fakeRepo) CreateSource(context.Context, catalogsync.Source) (string, error) {
	return "new", nil
}
func (f *fakeRepo) UpdateSource(context.Context, catalogsync.Source) error { return nil }
func (f *fakeRepo) DeleteSource(_ context.Context, _, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeRepo) ScheduleNext(context.Context, string) error {
	f.rescheduled++
	return nil
}
func (f *fakeRepo) StartRun(_ context.Context, sourceID, _, actor string, dry bool) (string, error) {
	if f.startErr != nil {
		return "", f.startErr
	}
	f.runs = append(f.runs, catalogsync.Run{
		ID: "run", SourceID: sourceID, Actor: actor, Dry: dry,
		Status: catalogsync.RunRunning, StartedAt: time.Now(),
	})
	return "run", nil
}
func (f *fakeRepo) FinishRun(_ context.Context, _, status, digest, _, errMsg string) error {
	f.finished = append(f.finished, status+"|"+digest+"|"+errMsg)
	if n := len(f.runs); n > 0 {
		f.runs[n-1].Status = status
		f.runs[n-1].DocumentSHA = digest
		f.runs[n-1].Error = errMsg
	}
	return nil
}
func (f *fakeRepo) ListRuns(context.Context, string, int32) ([]catalogsync.Run, error) {
	if len(f.runs) == 0 {
		return nil, nil
	}
	return []catalogsync.Run{f.runs[len(f.runs)-1]}, nil
}
func (f *fakeRepo) LastAppliedDigest(context.Context, string) (string, error) {
	return f.lastDigest, nil
}

type fakeFetcher struct {
	doc string
	err error
	n   int
}

func (f *fakeFetcher) Fetch(context.Context, catalogsync.Source) (string, error) {
	f.n++
	return f.doc, f.err
}

type fakeApplier struct {
	n       int
	lastDry bool
	err     error
}

func (f *fakeApplier) ApplyDocumentAsSystem(_ context.Context, _, _, _, _ string, dry bool) (string, int, error) {
	f.n++
	f.lastDry = dry
	if f.err != nil {
		return "", 0, f.err
	}
	return `{"sections":["roles"]}`, 7, nil
}

type fakeApps struct{}

func (fakeApps) ApplicationBySlug(context.Context, string, string) (*tenancydomain.ApplicationRecord, error) {
	return &tenancydomain.ApplicationRecord{ID: "app", Slug: "billing"}, nil
}

type fakeAudit struct{ events []string }

func (f *fakeAudit) Emit(_ context.Context, ev auditdomain.AuditEvent) {
	f.events = append(f.events, ev.Action)
}

func newTest(repo *fakeRepo, fetch *fakeFetcher, apply *fakeApplier) *syncInteractor {
	return &syncInteractor{
		repo: repo, fetch: fetch, apply: apply, audit: &fakeAudit{},
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		nowFunc: time.Now,
	}
}

func scheduled(id string) catalogsync.Source {
	return catalogsync.Source{
		ID: id, TenantID: "t", ApplicationSlug: "billing", Kind: catalogsync.KindHTTP,
		Format: FormatCSV, Status: catalogsync.StatusActive, IntervalSeconds: 900,
	}
}

// --- tests ------------------------------------------------------------------

// The digest skip is the difference between a five-minute poll and a
// five-minute rebuild of every gate snapshot in the installation.
func TestUnchangedDocumentIsSkipped(t *testing.T) {
	doc := "role,permissions\nviewer,invoice:read\n"
	repo := &fakeRepo{sources: []catalogsync.Source{scheduled("s1")}}
	fetch := &fakeFetcher{doc: doc}
	apply := &fakeApplier{}
	u := newTest(repo, fetch, apply)

	// First run installs it and records the digest.
	if _, err := u.run(context.Background(), repo.sources[0], "system", false); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if apply.n != 1 {
		t.Fatalf("applies = %d, want 1", apply.n)
	}
	repo.lastDigest = repo.runs[len(repo.runs)-1].DocumentSHA

	// Second run sees the same bytes and must not touch the catalog.
	if _, err := u.run(context.Background(), repo.sources[0], "system", false); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if apply.n != 1 {
		t.Errorf("applies = %d, want the unchanged document to be skipped", apply.n)
	}
	if got := repo.runs[len(repo.runs)-1].Status; got != catalogsync.RunSkipped {
		t.Errorf("status = %q, want %q", got, catalogsync.RunSkipped)
	}
	if repo.rescheduled != 2 {
		t.Errorf("rescheduled = %d, want a skipped run to still move the clock", repo.rescheduled)
	}
}

// A feed that is down must leave evidence and must not become a hot loop.
func TestFetchFailureIsRecordedAndRescheduled(t *testing.T) {
	repo := &fakeRepo{sources: []catalogsync.Source{scheduled("s1")}}
	u := newTest(repo, &fakeFetcher{err: apperr.ErrUnavailableFeed}, &fakeApplier{})

	if _, err := u.run(context.Background(), repo.sources[0], "system", false); err == nil {
		t.Fatal("want the failure returned to the caller")
	}
	if len(repo.finished) != 1 {
		t.Fatalf("finished = %v, want one closed run", repo.finished)
	}
	if got := repo.runs[0].Status; got != catalogsync.RunFailed {
		t.Errorf("status = %q, want %q", got, catalogsync.RunFailed)
	}
	if repo.runs[0].Error == "" {
		t.Error("a failed run has to say why")
	}
	if repo.rescheduled != 1 {
		t.Error("a source whose feed is down must still move on, or it hot-loops")
	}
}

// An apply that fails rolls its own rows back; the record that it was tried
// is written outside that transaction and must survive.
func TestApplyFailureIsRecorded(t *testing.T) {
	repo := &fakeRepo{sources: []catalogsync.Source{scheduled("s1")}}
	apply := &fakeApplier{err: errors.New("permission missing resource/action")}
	u := newTest(repo, &fakeFetcher{doc: "resource,action\n,\n"}, apply)

	if _, err := u.run(context.Background(), repo.sources[0], "system", false); err == nil {
		t.Fatal("want the failure returned")
	}
	if got := repo.runs[0].Status; got != catalogsync.RunFailed {
		t.Errorf("status = %q, want %q", got, catalogsync.RunFailed)
	}
	if repo.runs[0].DocumentSHA == "" {
		t.Error("a failed apply still knows which document it was")
	}
}

// A dry run proves a document parses. It must not move the clock, and must
// not be mistaken for something that installed rows.
func TestDryRunDoesNotSchedule(t *testing.T) {
	repo := &fakeRepo{sources: []catalogsync.Source{scheduled("s1")}}
	apply := &fakeApplier{}
	u := newTest(repo, &fakeFetcher{doc: "role\nviewer\n"}, apply)

	if _, err := u.run(context.Background(), repo.sources[0], "op-1", true); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !apply.lastDry {
		t.Error("the apply was not told it was a dry run")
	}
	if got := repo.runs[0].Status; got != catalogsync.RunDry {
		t.Errorf("status = %q, want %q", got, catalogsync.RunDry)
	}
	if repo.rescheduled != 0 {
		t.Error("a dry run must not move a source's due time")
	}
}

// A dry run never skips: the point is to prove the document parses, and the
// digest only says the bytes are the same, not that the rows are installed.
func TestDryRunIgnoresTheDigest(t *testing.T) {
	doc := "role\nviewer\n"
	repo := &fakeRepo{sources: []catalogsync.Source{scheduled("s1")}}
	apply := &fakeApplier{}
	u := newTest(repo, &fakeFetcher{doc: doc}, apply)
	if _, err := u.run(context.Background(), repo.sources[0], "op-1", false); err != nil {
		t.Fatalf("first: %v", err)
	}
	repo.lastDigest = repo.runs[0].DocumentSHA

	if _, err := u.run(context.Background(), repo.sources[0], "op-1", true); err != nil {
		t.Fatalf("dry: %v", err)
	}
	if apply.n != 2 {
		t.Errorf("applies = %d, want the dry run to go through anyway", apply.n)
	}
}

// One bad feed must not stop the rest of the installation's sources.
func TestRunDueContinuesPastAFailure(t *testing.T) {
	repo := &fakeRepo{sources: []catalogsync.Source{scheduled("s1"), scheduled("s2")}}
	fetch := &fakeFetcher{err: apperr.ErrUnavailableFeed}
	u := newTest(repo, fetch, &fakeApplier{})

	ran, err := u.RunDue(context.Background(), time.Now(), 0)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if ran != 2 || fetch.n != 2 {
		t.Fatalf("ran = %d, fetches = %d, want both sources attempted", ran, fetch.n)
	}
}

// A scheduled run has no person behind it and must not borrow one.
func TestScheduledRunIsAuditedAsSystem(t *testing.T) {
	repo := &fakeRepo{sources: []catalogsync.Source{scheduled("s1")}}
	u := newTest(repo, &fakeFetcher{doc: "role\nviewer\n"}, &fakeApplier{})
	if _, err := u.run(context.Background(), repo.sources[0], catalogsync.ActorSystem, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := repo.runs[0].Actor; got != catalogsync.ActorSystem {
		t.Errorf("actor = %q, want %q", got, catalogsync.ActorSystem)
	}
}

// A manual source is one an operator runs; nothing should move its clock,
// because it has none.
func TestManualSourceIsNeverRescheduled(t *testing.T) {
	src := scheduled("s1")
	src.IntervalSeconds = 0
	repo := &fakeRepo{sources: []catalogsync.Source{src}}
	u := newTest(repo, &fakeFetcher{doc: "role\nviewer\n"}, &fakeApplier{})
	if _, err := u.run(context.Background(), src, "op-1", false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if repo.rescheduled != 0 {
		t.Error("a manual source has no due time to move")
	}
}
