package main

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	"github.com/gsoultan/anubis/internal/config"
	"github.com/gsoultan/anubis/internal/crypto/keyring"
	"github.com/gsoultan/anubis/internal/ratelimit"
	"github.com/gsoultan/anubis/internal/repository"
	"github.com/gsoultan/anubis/internal/repository/postgres"
	"github.com/gsoultan/anubis/internal/snapshot"
	"github.com/gsoultan/anubis/internal/transport/httpapi"
	"github.com/gsoultan/anubis/internal/usecase"
)

func registerAdminHandlers(mux *http.ServeMux, opts connect.HandlerOption,
	store *postgres.Store, auditor *postgres.ChainedAuditor, master []byte,
	logger *slog.Logger) {
	adminWiring{store: store, auditor: auditor, master: master, logger: logger}.register(mux, opts)
}

// registerBrowserRoutes mounts the OIDC PKCE flow and the hosted login page.
func registerBrowserRoutes(srv *httpapi.Server, cfg *config.Config, store *postgres.Store,
	_ *keyring.Manager, issuer usecase.TokenIssuer, clock repository.Clock,
	auditor repository.Auditor, limiter *ratelimit.Limiter, logger *slog.Logger) {
	oidc := httpapi.NewOIDCHandler(cfg.Issuer,
		store, store, store, store, store, store, store, store,
		issuer, clock, auditor, limiter, logger)
	srv.HandleFunc("GET /v1/authorize", oidc.Authorize)
	srv.HandleFunc("POST /v1/login", oidc.LoginForm)
	srv.HandleFunc("POST /v1/token", oidc.Token)
}

// registerGate starts the snapshot manager and mounts /v1/gate/check.
func registerGate(ctx context.Context, srv *httpapi.Server, cfg *config.Config,
	_ interface{ Close() }, store *postgres.Store, ring *keyring.Manager,
	_ repository.Clock, logger *slog.Logger) {
	snaps := snapshot.NewManager(store, store, logger)
	go snaps.Run(ctx)
	gate := httpapi.NewGateHandler(cfg.Issuer, ring, snaps)
	srv.HandleFunc("POST /v1/gate/check", gate.Check)
	srv.HandleFunc("GET /v1/gate/check", gate.Check)
}
