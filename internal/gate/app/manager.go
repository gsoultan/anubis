package gateapp

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gsoultan/anubis/internal/gate/snapshot"
	"github.com/gsoultan/anubis/internal/platform/metrics"
)

// Loader is what the Manager needs from the repository layer.
type Loader interface {
	LoadSnapshot(ctx context.Context, tenantID, tenantSlug string, revokedWindow time.Duration) (*snapshot.Data, error)
	// CatalogVersion is the cheap "has anything changed" probe.
	CatalogVersion(ctx context.Context, tenantID string) (int64, error)
	WatchCatalog(ctx context.Context, onBump func(tenantID string)) error
}

// TenantLister enumerates active tenants for the refresh loop.
type TenantLister interface {
	Tenants(ctx context.Context) (ids []string, slugs []string, err error)
}

// Manager keeps per-tenant snapshots fresh: LISTEN/NOTIFY push plus interval
// polling as the correctness backstop. Reads are lock-free per request
// (RWMutex around a map swap).
type Manager struct {
	loader        Loader
	tenants       TenantLister
	logger        *slog.Logger
	pollInterval  time.Duration
	maxAge        time.Duration
	revokedWindow time.Duration
	// A catalog bump reloads a tenant's WHOLE snapshot, which is the most
	// expensive thing this process does. The statement-level triggers in
	// migration 0006 already collapse a bulk write into one bump, but a sync
	// that issues many separate statements still produces a bump per
	// statement — and reloading once per statement means the reloads, not the
	// sync, become the bottleneck. Coalesce: wait for quiet, but never longer
	// than coalesceMax, so a continuous stream still refreshes promptly.
	coalesceQuiet time.Duration
	coalesceMax   time.Duration
	// rebuildEvery bounds how long the version gate may be trusted. See load.
	rebuildEvery time.Duration

	mu    sync.RWMutex
	data  map[string]*snapshot.Data // by tenant slug
	dirty chan string
}

func NewManager(loader Loader, tenants TenantLister, maxAge time.Duration, logger *slog.Logger) *Manager {
	if maxAge <= 0 {
		maxAge = 5 * time.Minute
	}
	return &Manager{
		loader: loader, tenants: tenants, logger: logger,
		pollInterval:  30 * time.Second,
		maxAge:        maxAge,
		revokedWindow: 30 * time.Minute,
		coalesceQuiet: 250 * time.Millisecond,
		coalesceMax:   2 * time.Second,
		// Once per max-age window, prove the snapshot from scratch. Deliberately
		// the SAME knob that defines staleness: it makes the guarantee one an
		// operator already reasons about — nothing is served that has not been
		// fully rebuilt within max age — rather than a second, subtly different
		// freshness bound to hold in your head.
		rebuildEvery: maxAge,
		data:         map[string]*snapshot.Data{},
		dirty:        make(chan string, 64),
	}
}

// Stale reports the freshness of the OLDEST loaded tenant snapshot, which is
// what readiness must judge on: one stale tenant means this instance is
// failing closed for that tenant's traffic.
func (m *Manager) Stale() (bool, time.Duration, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.data) == 0 {
		return true, 0, false
	}
	var worst time.Duration
	for _, d := range m.data {
		if age := time.Since(d.LoadedAt); age > worst {
			worst = age
		}
	}
	return worst > m.maxAge, worst, true
}

// Get returns the snapshot for a tenant slug, and whether it is FRESH enough
// to serve authorization from. Stale-but-present implements fail-static;
// past maxAge the gate must fail closed.
func (m *Manager) Get(tenantSlug string) (d *snapshot.Data, fresh bool) {
	m.mu.RLock()
	d = m.data[tenantSlug]
	m.mu.RUnlock()
	if d == nil {
		return nil, false
	}
	return d, time.Since(d.LoadedAt) <= m.maxAge
}

// Run blocks: initial load, then LISTEN + poll until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	m.refreshAll(ctx)

	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			if err := m.loader.WatchCatalog(ctx, func(tenantID string) {
				select {
				case m.dirty <- tenantID:
				default: // refresh already pending; the poll will catch up
				}
			}); err != nil && ctx.Err() == nil {
				m.logger.Warn("catalog listener dropped; relying on poll", "error", err)
				select {
				case <-time.After(5 * time.Second):
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()
	coalesce := time.NewTimer(time.Hour)
	coalesce.Stop()
	defer coalesce.Stop()

	pending := map[string]struct{}{}
	var deadline time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// The poll refreshes everything, but pending is NOT cleared: a
			// bump that landed mid-pass may describe a commit this pass did
			// not see, and dropping it would lose that change until the next
			// poll.
			m.refreshAll(ctx)
		case tenantID := <-m.dirty:
			if len(pending) == 0 {
				deadline = time.Now().Add(m.coalesceMax)
			}
			pending[tenantID] = struct{}{}
			wait := m.coalesceQuiet
			if until := time.Until(deadline); until < wait {
				wait = max(until, 0)
			}
			coalesce.Reset(wait)
		case <-coalesce.C:
			m.refreshPending(ctx, pending)
		}
	}
}

func (m *Manager) refreshAll(ctx context.Context) {
	ids, slugs, err := m.tenants.Tenants(ctx)
	if err != nil {
		m.logger.Warn("snapshot: tenant list failed", "error", err)
		return
	}
	for i := range ids {
		m.load(ctx, ids[i], slugs[i])
	}
}

// refreshPending reloads every tenant bumped since the last pass, resolving
// the tenant list ONCE for the whole batch rather than once per bump.
// Consumes pending on success; on a tenant-list failure it leaves the set
// intact so the next bump or the poll retries.
func (m *Manager) refreshPending(ctx context.Context, pending map[string]struct{}) {
	if len(pending) == 0 {
		return
	}
	ids, slugs, err := m.tenants.Tenants(ctx)
	if err != nil {
		m.logger.Warn("snapshot: tenant list failed", "error", err)
		return
	}
	for i := range ids {
		if _, ok := pending[ids[i]]; ok {
			m.load(ctx, ids[i], slugs[i])
		}
	}
	// Anything still here names a tenant the lister no longer returns, which
	// no amount of retrying will load.
	clear(pending)
}

// load refreshes one tenant, rebuilding only if the catalog actually moved.
//
// The poll runs every 30 s over every tenant, and a rebuild is ~92 MB at a
// million scope nodes — so on an idle tenant this is the difference between
// a full reload and one indexed row. It is sound ONLY because every table
// that can change a decision bumps the catalog version (migrations 0005/0006
// and 0040). If a table is ever added to the snapshot without a trigger, this
// shortcut is what silently stops propagating it, which is why
// TestSnapshotTablesAreClassifiedPushOrPoll exists.
//
// A version-probe failure falls through to the rebuild: the expensive path is
// always the safe one.
func (m *Manager) load(ctx context.Context, tenantID, slug string) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	outcome := "rebuilt"
	if cur, _ := m.Get(slug); cur != nil {
		if time.Since(cur.BuiltAt) >= m.rebuildEvery {
			// Periodic proof from scratch. The version gate is only as good as
			// the invalidation triggers behind it, and a MISSING trigger is
			// invisible at runtime — the version simply never moves and the
			// gate serves stale authorization indefinitely. Rebuilding once
			// per max-age window turns that from unbounded into bounded by the
			// same window operators already watch, and still skips ~90% of
			// rebuilds at the default 30s poll.
			outcome = "verify"
		} else if v, err := m.loader.CatalogVersion(ctx, tenantID); err == nil && v == cur.Version {
			m.touch(slug, cur)
			metrics.IncSnapshotRefresh(slug, "unchanged")
			return
		}
	}

	d, err := m.loader.LoadSnapshot(ctx, tenantID, slug, m.revokedWindow)
	if err != nil {
		metrics.IncSnapshotRefresh(slug, "failed")
		m.logger.Error("snapshot load failed (serving previous, fail-static)",
			"tenant", slug, "error", err)
		return
	}
	m.mu.Lock()
	m.data[slug] = d
	m.mu.Unlock()
	metrics.SetSnapshotLoaded(slug, time.Now())
	metrics.SetSnapshotNodes(slug, d.Scope.Len())
	metrics.IncSnapshotRefresh(slug, outcome)
	m.logger.Info("snapshot loaded", "tenant", slug, "version", d.Version,
		"reason", outcome, "scope_nodes", d.Scope.Len(),
		"grants", len(d.GrantsByIdentity), "routes", len(d.Routes))
}

// touch marks an unchanged snapshot as current without rebuilding it.
//
// It publishes a COPY rather than assigning through the existing pointer:
// readers hold *Data and read LoadedAt without a lock, so writing that field
// in place is a data race. The maps and the scope index are immutable after
// load, so the copy shares them and costs a few words.
//
// RevokedSessions is queried over a sliding window, so skipping the rebuild
// leaves entries in it past their window. That is fail-CLOSED — the extra
// entries only deny sessions already too old for any live token — and the
// next real change rebuilds and drops them.
func (m *Manager) touch(slug string, cur *snapshot.Data) {
	fresh := *cur
	fresh.LoadedAt = time.Now()

	m.mu.Lock()
	if m.data[slug] == cur { // lost the race to a real reload; that one wins
		m.data[slug] = &fresh
	}
	m.mu.Unlock()
	metrics.SetSnapshotLoaded(slug, time.Now())
}
