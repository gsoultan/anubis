package apihttp

import (
	"context"
	"net/http"
	"time"

	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthHandler serves /healthz (process alive) and /readyz (database
// reachable, active signing key present).
type HealthHandler struct {
	pool *pgxpool.Pool
	ring *keyring.Manager
}

func NewHealthHandler(pool *pgxpool.Pool, ring *keyring.Manager) *HealthHandler {
	return &HealthHandler{pool: pool, ring: ring}
}

func (h *HealthHandler) Healthz(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	checks := map[string]string{}
	ok := true

	if err := h.pool.Ping(ctx); err != nil {
		checks["database"] = "unreachable"
		ok = false
	} else {
		checks["database"] = "ok"
	}
	if _, err := h.ring.Ring().ActiveAccess(); err != nil {
		checks["signing_key"] = "missing"
		ok = false
	} else {
		checks["signing_key"] = "ok"
	}
	status := http.StatusOK
	if !ok {
		status = http.StatusServiceUnavailable
	}
	WriteJSON(w, status, checks)
}
