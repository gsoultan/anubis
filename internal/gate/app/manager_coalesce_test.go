package gateapp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gsoultan/anubis/internal/gate/snapshot"
	"github.com/gsoultan/anubis/internal/platform/metrics"
	"net/http/httptest"
	"strings"
)

// A catalog bump reloads a tenant's entire snapshot. At a million scope nodes
// that is the most expensive thing the process does, so a sync that writes N
// statements must not cost N reloads — otherwise the reloads outlast the sync
// and the gate spends its time rebuilding state nobody asked for yet.

type countingLoader struct {
	mu      sync.Mutex
	loads   int
	version int64
	probes  int
	verr    error
	loaded  chan struct{}
}

func (l *countingLoader) LoadSnapshot(_ context.Context, _, _ string, _ time.Duration) (*snapshot.Data, error) {
	l.mu.Lock()
	l.loads++
	v := l.version
	l.mu.Unlock()
	select {
	case l.loaded <- struct{}{}:
	default:
	}
	now := time.Now()
	return &snapshot.Data{LoadedAt: now, BuiltAt: now, Version: v}, nil
}

// CatalogVersion is the cheap probe the Manager uses to avoid rebuilding.
func (l *countingLoader) CatalogVersion(context.Context, string) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.probes++
	return l.version, l.verr
}

// bump models a real catalog change: the version moves, so the next refresh
// must actually rebuild.
func (l *countingLoader) bump() {
	l.mu.Lock()
	l.version++
	l.mu.Unlock()
}

func (l *countingLoader) probeCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.probes
}

func (l *countingLoader) WatchCatalog(ctx context.Context, _ func(string)) error {
	<-ctx.Done()
	return ctx.Err()
}

func (l *countingLoader) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loads
}

type oneTenant struct{}

func (oneTenant) Tenants(context.Context) ([]string, []string, error) {
	return []string{"t1"}, []string{"acme"}, nil
}

func startManager(t *testing.T, quiet, maxWait time.Duration) (*Manager, *countingLoader) {
	t.Helper()
	l := &countingLoader{loaded: make(chan struct{}, 64)}
	m := NewManager(l, oneTenant{}, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.pollInterval = time.Hour // the poll is the backstop; keep it out of this
	m.coalesceQuiet = quiet
	m.coalesceMax = maxWait

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.Run(ctx)

	select { // the initial refreshAll
	case <-l.loaded:
	case <-time.After(2 * time.Second):
		t.Fatal("initial snapshot load never happened")
	}
	return m, l
}

func TestABurstOfBumpsCostsOneReload(t *testing.T) {
	quiet := 40 * time.Millisecond
	m, l := startManager(t, quiet, 2*time.Second)

	for i := 0; i < 25; i++ {
		l.bump() // a real catalog change, or the version gate rightly skips it
		m.dirty <- "t1"
	}
	select {
	case <-l.loaded:
	case <-time.After(2 * time.Second):
		t.Fatal("the coalesced reload never happened")
	}
	time.Sleep(4 * quiet) // any un-coalesced reloads would land in this window

	if got := l.count(); got != 2 {
		t.Errorf("25 bumps caused %d loads, want 2 (one initial + one coalesced)", got)
	}
}

// The quiet window must not be resettable forever: a steady stream of writes
// would otherwise starve the refresh and the gate would serve stale data for
// as long as the writes continued.
func TestAContinuousStreamStillRefreshes(t *testing.T) {
	m, l := startManager(t, 60*time.Millisecond, 100*time.Millisecond)

	stop := time.After(400 * time.Millisecond)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for done := false; !done; {
		select {
		case <-tick.C:
			l.bump()
			m.dirty <- "t1"
		case <-stop:
			done = true
		}
	}
	// Bumps arrived every 10ms, so the 60ms quiet window never expired on its
	// own; only the 100ms ceiling can have fired.
	if got := l.count(); got < 2 {
		t.Errorf("continuous bumps produced %d loads, want at least 2 — the ceiling never fired", got)
	}
}

// A bumped tenant the lister no longer returns must not be retried forever.
func TestAVanishedTenantIsDropped(t *testing.T) {
	m, l := startManager(t, 20*time.Millisecond, time.Second)
	before := l.count()

	pending := map[string]struct{}{"ghost": {}}
	m.refreshPending(context.Background(), pending)

	if len(pending) != 0 {
		t.Errorf("pending still holds %d entries; a deleted tenant would be retried forever", len(pending))
	}
	if got := l.count(); got != before {
		t.Errorf("loaded %d times for a tenant that does not exist", got-before)
	}
}

// The poll walks every tenant every 30s and a rebuild is ~92 MB at a million
// scope nodes. Rebuilding a tenant whose catalog has not moved is the whole
// cost of carrying idle tenants, so it must not happen.
func TestAnUnchangedTenantIsNotRebuilt(t *testing.T) {
	m, l := startManager(t, 20*time.Millisecond, time.Second)
	before := l.count()

	for i := 0; i < 5; i++ {
		m.refreshAll(context.Background())
	}
	if got := l.count(); got != before {
		t.Errorf("rebuilt %d times with an unchanged catalog version", got-before)
	}
	if l.probeCount() < 5 {
		t.Errorf("only %d version probes for 5 refreshes — the cheap path is not being taken", l.probeCount())
	}
}

// ...but the snapshot must still count as FRESH, or readiness fails and the
// gate starts denying on data that is provably current.
func TestSkippingTheRebuildStillRefreshesFreshness(t *testing.T) {
	m, _ := startManager(t, 20*time.Millisecond, time.Second)
	m.maxAge = 40 * time.Millisecond

	time.Sleep(60 * time.Millisecond) // let it go stale
	if _, fresh := m.Get("acme"); fresh {
		t.Fatal("precondition: the snapshot should have gone stale")
	}
	m.refreshAll(context.Background())
	if _, fresh := m.Get("acme"); !fresh {
		t.Error("an unchanged snapshot stayed stale after a refresh — readiness would fail " +
			"and the gate would deny on data it had just confirmed current")
	}
}

func TestAChangedVersionStillRebuilds(t *testing.T) {
	m, l := startManager(t, 20*time.Millisecond, time.Second)
	before := l.count()

	l.bump()
	m.refreshAll(context.Background())
	if got := l.count(); got != before+1 {
		t.Errorf("a moved catalog version produced %d rebuilds, want 1", got-before)
	}
	// and the new version is what subsequent probes compare against
	m.refreshAll(context.Background())
	if got := l.count(); got != before+1 {
		t.Errorf("rebuilt again with no further change (%d total)", got-before)
	}
}

// If the probe itself fails we must fall through to the rebuild. The
// expensive path is the safe one; treating a probe error as "unchanged" would
// pin the gate to whatever it last loaded.
func TestAFailedVersionProbeFallsBackToReload(t *testing.T) {
	m, l := startManager(t, 20*time.Millisecond, time.Second)
	before := l.count()

	l.mu.Lock()
	l.verr = errProbe
	l.mu.Unlock()

	m.refreshAll(context.Background())
	if got := l.count(); got != before+1 {
		t.Errorf("a failed version probe produced %d reloads, want 1", got-before)
	}
}

var errProbe = errors.New("probe failed")

// The version gate is only as good as the invalidation triggers behind it,
// and a MISSING trigger is invisible at runtime: the version never moves, so
// the gate skips the rebuild forever and serves stale authorization with
// nothing in the logs to say so. A periodic rebuild bounds that to one
// max-age window — the same window operators already watch.
func TestTheSnapshotIsRebuiltFromScratchPeriodically(t *testing.T) {
	m, l := startManager(t, 20*time.Millisecond, time.Second)
	m.rebuildEvery = 60 * time.Millisecond
	before := l.count()

	// Nothing has changed, so these must all take the cheap path.
	for i := 0; i < 3; i++ {
		m.refreshAll(context.Background())
	}
	if got := l.count(); got != before {
		t.Fatalf("rebuilt %d times inside the window with an unchanged version", got-before)
	}

	time.Sleep(80 * time.Millisecond) // BuiltAt now older than rebuildEvery
	m.refreshAll(context.Background())
	if got := l.count(); got != before+1 {
		t.Errorf("no rebuild after %v with an unchanged version — a lost trigger "+
			"would keep this snapshot alive indefinitely", m.rebuildEvery)
	}

	// And the clock restarts: the next refresh is cheap again.
	m.refreshAll(context.Background())
	if got := l.count(); got != before+1 {
		t.Errorf("rebuilt again immediately (%d total); BuiltAt was not reset", got-before)
	}
}

// A periodic rebuild must not be silent either — an operator watching
// anubis_gate_snapshot_refresh_total needs to tell the three cases apart.
func TestRefreshOutcomesAreDistinguishable(t *testing.T) {
	m, l := startManager(t, 20*time.Millisecond, time.Second)
	m.rebuildEvery = time.Hour // keep the periodic rebuild out of this

	m.refreshAll(context.Background()) // unchanged
	l.bump()
	m.refreshAll(context.Background()) // rebuilt

	body := metricsBody(t)
	for _, want := range []string{
		`anubis_gate_snapshot_refresh_total{tenant="acme",result="unchanged"}`,
		`anubis_gate_snapshot_refresh_total{tenant="acme",result="rebuilt"}`,
		`anubis_gate_snapshot_scope_nodes{tenant="acme"}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics is missing %s", want)
		}
	}
}

func metricsBody(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	return rec.Body.String()
}

// The version gate must never be able to keep a STALE snapshot alive.
//
// This falls out of tying rebuildEvery to maxAge rather than picking a second
// number: touch() only ever moves LoadedAt forward and leaves BuiltAt alone,
// so BuiltAt <= LoadedAt always. A snapshot old enough to be stale therefore
// has a BuiltAt at least that old, which forces the rebuild path before the
// version is even consulted. Give the two knobs independent values and this
// stops being true — a snapshot could be refreshed to "fresh" by a version
// probe without anyone re-reading the database.
func TestAStaleSnapshotIsAlwaysRebuiltNotTouched(t *testing.T) {
	m, l := startManager(t, 20*time.Millisecond, time.Second)
	m.maxAge = 40 * time.Millisecond
	m.rebuildEvery = m.maxAge
	before := l.count()

	time.Sleep(60 * time.Millisecond)
	if _, fresh := m.Get("acme"); fresh {
		t.Fatal("precondition: the snapshot should have gone stale")
	}

	m.refreshAll(context.Background()) // version is UNCHANGED throughout
	if got := l.count(); got != before+1 {
		t.Fatalf("a stale snapshot was revived by the version gate without a rebuild (%d loads)", got-before)
	}
	if _, fresh := m.Get("acme"); !fresh {
		t.Error("the rebuild did not restore freshness")
	}
}

// And the guarantee is structural, not incidental: whatever the knobs, a
// snapshot is never served as fresh when it has not been rebuilt within the
// staleness window.
func TestRebuildIntervalDoesNotExceedMaxAge(t *testing.T) {
	for _, maxAge := range []time.Duration{time.Minute, 5 * time.Minute, time.Hour} {
		m := NewManager(nil, nil, maxAge, nil)
		if m.rebuildEvery > m.maxAge {
			t.Errorf("maxAge=%v: rebuildEvery=%v exceeds it, so a snapshot could be "+
				"reported fresh without having been rebuilt in that window", maxAge, m.rebuildEvery)
		}
	}
}
