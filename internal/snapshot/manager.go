package snapshot

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Loader is what the Manager needs from the repository layer.
type Loader interface {
	LoadSnapshot(ctx context.Context, tenantID, tenantSlug string, revokedWindow time.Duration) (*Data, error)
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

	mu   sync.RWMutex
	data map[string]*Data // by tenant slug
	dirty chan string
}

func NewManager(loader Loader, tenants TenantLister, logger *slog.Logger) *Manager {
	return &Manager{
		loader: loader, tenants: tenants, logger: logger,
		pollInterval:  30 * time.Second,
		maxAge:        5 * time.Minute,
		revokedWindow: 30 * time.Minute,
		data:          map[string]*Data{},
		dirty:         make(chan string, 64),
	}
}

// Get returns the snapshot for a tenant slug, and whether it is FRESH enough
// to serve authorization from. Stale-but-present implements fail-static;
// past maxAge the gate must fail closed.
func (m *Manager) Get(tenantSlug string) (d *Data, fresh bool) {
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
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.refreshAll(ctx)
		case tenantID := <-m.dirty:
			m.refreshOne(ctx, tenantID)
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

func (m *Manager) refreshOne(ctx context.Context, tenantID string) {
	ids, slugs, err := m.tenants.Tenants(ctx)
	if err != nil {
		return
	}
	for i := range ids {
		if ids[i] == tenantID {
			m.load(ctx, ids[i], slugs[i])
			return
		}
	}
}

func (m *Manager) load(ctx context.Context, tenantID, slug string) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	d, err := m.loader.LoadSnapshot(ctx, tenantID, slug, m.revokedWindow)
	if err != nil {
		m.logger.Error("snapshot load failed (serving previous, fail-static)",
			"tenant", slug, "error", err)
		return
	}
	m.mu.Lock()
	m.data[slug] = d
	m.mu.Unlock()
	m.logger.Info("snapshot loaded", "tenant", slug, "version", d.Version,
		"grants", len(d.GrantsByIdentity), "routes", len(d.Routes))
}
