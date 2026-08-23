package apihttp

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
)

// SnapshotProbe reports how fresh the gate's authorization snapshot is.
// Implemented by the gate context; readiness must fail while the snapshot is
// too old to serve from, because past that age the gate fails closed and this
// instance is denying traffic it should be allowing.
type SnapshotProbe interface {
	Stale() (stale bool, age time.Duration, loaded bool)
}

// HealthHandler serves /healthz (process alive) and /readyz (dependencies
// usable). Kubernetes treats them differently on purpose: a failing readyz
// removes the instance from the load balancer, a failing healthz restarts it.
type HealthHandler struct {
	pool  *pgxpool.Pool
	ring  *keyring.Manager
	snaps SnapshotProbe
}

func NewHealthHandler(pool *pgxpool.Pool, ring *keyring.Manager) *HealthHandler {
	return &HealthHandler{pool: pool, ring: ring}
}

// WithSnapshot adds the gate's freshness probe once the gate is wired.
func (h *HealthHandler) WithSnapshot(p SnapshotProbe) { h.snaps = p }

func (h *HealthHandler) Healthz(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	checks := map[string]any{}
	ok := true

	if err := h.pool.Ping(ctx); err != nil {
		checks["database"] = "unreachable"
		ok = false
	} else {
		stat := h.pool.Stat()
		checks["database"] = map[string]any{
			"status": "ok", "acquired": stat.AcquiredConns(),
			"idle": stat.IdleConns(), "max": stat.MaxConns(),
		}
	}
	if _, err := h.ring.Ring().ActiveAccess(); err != nil {
		checks["signing_key"] = "missing"
		ok = false
	} else {
		checks["signing_key"] = "ok"
	}
	if h.snaps != nil {
		stale, age, loaded := h.snaps.Stale()
		switch {
		case !loaded:
			checks["snapshot"] = "never loaded"
			ok = false
		case stale:
			checks["snapshot"] = map[string]any{"status": "stale", "age": age.String()}
			ok = false
		default:
			checks["snapshot"] = map[string]any{"status": "ok", "age": age.String()}
		}
	}

	status := http.StatusOK
	if !ok {
		status = http.StatusServiceUnavailable
	}
	WriteJSON(w, status, checks)
}
