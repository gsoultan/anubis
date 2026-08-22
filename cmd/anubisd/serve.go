package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	"github.com/gsoultan/anubis/internal/config"
	"github.com/gsoultan/anubis/internal/endpoint"
	"github.com/gsoultan/anubis/internal/migrate"
	"github.com/gsoultan/anubis/internal/ratelimit"
	"github.com/gsoultan/anubis/internal/repository/postgres"
	"github.com/gsoultan/anubis/internal/service"
	"github.com/gsoultan/anubis/internal/transport/connectrpc"
	"github.com/gsoultan/anubis/internal/transport/httpapi"
	"github.com/gsoultan/anubis/internal/usecase"
	"github.com/gsoultan/anubis/migrations"
)

func runServe(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Dev convenience mirrors scripts/dev.sh expectations: schema is applied
	// on boot. Production runs `anubisd migrate` as its own deploy step.
	if cfg.Env != "prod" {
		conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		if _, err := migrate.NewRunner(migrations.FS, logger).Run(ctx, conn); err != nil {
			conn.Close(context.WithoutCancel(ctx))
			return err
		}
		conn.Close(context.WithoutCancel(ctx))
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	store := postgres.NewStore(pool)
	clock := systemClock{}

	ring, err := loadRing(ctx, logger, store, cfg.MasterKey, cfg.AutoKeys)
	if err != nil {
		return err
	}

	auditor := postgres.NewChainedAuditor(store, logger)
	defer auditor.Close()
	notifier := postgres.NewHTTPBackchannelNotifier(logger)
	limiter := ratelimit.New()

	// --- usecases -----------------------------------------------------------
	issuer := usecase.NewPasetoTokenIssuer(cfg.Issuer, ring, store, store, store, store, store, store, clock)
	login := usecase.NewLoginInteractor(store, store, store, store, store, store, issuer, ring, store, clock, auditor)
	verifyMfa := usecase.NewVerifyMfaInteractor(ring, store, store, store, store, store, store, issuer, store, clock, auditor)
	refresh := usecase.NewRefreshInteractor(store, store, store, issuer, store, auditor)
	backchannel := usecase.NewBackchannelLogout(cfg.Issuer, ring, store, notifier, clock)
	logout := usecase.NewLogoutInteractor(store, store, store, store, auditor, backchannel)
	device := usecase.NewDeviceInteractor(store, store, store, store, store, store, issuer, store, clock, auditor)
	register := usecase.NewRegisterInteractor(store, store, store, store, store, store, store, clock, auditor)
	verifyEmail := usecase.NewVerifyEmailInteractor(store, store)
	introspect := usecase.NewIntrospectInteractor(cfg.Issuer, ring, store, store, clock)
	revoke := usecase.NewRevokeInteractor(store, store, store, auditor)
	authorize := usecase.NewAuthorizeInteractor(store, clock, auditor)
	explain := usecase.NewExplainInteractor(store)
	switchScope := usecase.NewSwitchScopeInteractor(store, store, store, issuer, store, auditor)
	getMe := usecase.NewGetMeInteractor(store, store)
	listSessions := usecase.NewListSessionsInteractor(store)

	// --- services -----------------------------------------------------------
	authSvc := service.NewAuthService(login, verifyMfa, refresh,
		logout, logout.All(), logout.Session(),
		device, device.Verify(), register, verifyEmail)
	tokenSvc := service.NewTokenService(introspect, revoke)
	authzSvc := service.NewAuthzService(authorize, explain, switchScope)
	sessionSvc := service.NewSessionService(getMe, listSessions, logout.Session())

	// --- endpoints ----------------------------------------------------------
	authEps := endpoint.NewAuthEndpoints(authSvc, logger, limiter)
	tokenEps := endpoint.NewTokenEndpoints(tokenSvc, logger)
	authzEps := endpoint.NewAuthzEndpoints(authzSvc, logger)
	sessionEps := endpoint.NewSessionEndpoints(sessionSvc, logger)

	// --- transports ---------------------------------------------------------
	opts := connect.WithInterceptors(
		connectrpc.NewMetaInterceptor(),
		connectrpc.NewAuthnInterceptor(cfg.Issuer, ring, store, store, clock),
	)
	rpc := http.NewServeMux()
	rpc.Handle(anubisv1connect.NewAuthServiceHandler(connectrpc.NewAuthHandler(authEps), opts))
	rpc.Handle(anubisv1connect.NewTokenServiceHandler(connectrpc.NewTokenHandler(tokenEps), opts))
	rpc.Handle(anubisv1connect.NewAuthzServiceHandler(connectrpc.NewAuthzHandler(authzEps), opts))
	rpc.Handle(anubisv1connect.NewSessionServiceHandler(connectrpc.NewSessionHandler(sessionEps), opts))
	registerAdminHandlers(rpc, opts, store, ring, clock, logger, auditor)

	srv := httpapi.NewServer(logger, cfg.UIOrigin, rpc,
		httpapi.NewHealthHandler(pool, ring),
		httpapi.NewWellKnownHandler(cfg.Issuer, ring))
	registerBrowserRoutes(srv, cfg, store, ring, issuer, clock, auditor, limiter, logger)
	registerGate(ctx, srv, cfg, pool, store, ring, clock, logger)

	httpSrv := srv.HTTPServer(cfg.Listen)

	if cfg.DebugListen != "" {
		dbg := httpapi.DebugServer(cfg.DebugListen)
		go func() {
			logger.Info("debug listener", "addr", cfg.DebugListen)
			_ = dbg.ListenAndServe()
		}()
		defer dbg.Close()
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("anubis api listening", "addr", cfg.Listen, "issuer", cfg.Issuer, "env", cfg.Env)
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		logger.Info("shutting down")
		return httpSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
