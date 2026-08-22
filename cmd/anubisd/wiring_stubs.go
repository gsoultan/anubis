package main

// Wiring points filled by their feature batches: admin plane handlers,
// browser (OIDC) routes, and the gate. Each batch replaces its stub.

import (
	"context"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gsoultan/anubis/internal/config"
	"github.com/gsoultan/anubis/internal/crypto/keyring"
	"github.com/gsoultan/anubis/internal/ratelimit"
	"github.com/gsoultan/anubis/internal/repository"
	"github.com/gsoultan/anubis/internal/repository/postgres"
	"github.com/gsoultan/anubis/internal/transport/httpapi"
	"github.com/gsoultan/anubis/internal/usecase"
)

func registerAdminHandlers(_ *http.ServeMux, _ connect.HandlerOption,
	_ *postgres.Store, _ *keyring.Manager, _ repository.Clock, _ *slog.Logger,
	_ repository.Auditor) {
}

func registerBrowserRoutes(_ *httpapi.Server, _ *config.Config, _ *postgres.Store,
	_ *keyring.Manager, _ usecase.TokenIssuer, _ repository.Clock,
	_ repository.Auditor, _ *ratelimit.Limiter, _ *slog.Logger) {
}

func registerGate(_ context.Context, _ *httpapi.Server, _ *config.Config,
	_ *pgxpool.Pool, _ *postgres.Store, _ *keyring.Manager, _ repository.Clock,
	_ *slog.Logger) {
}
