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
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	apihttp "github.com/gsoultan/anubis/internal/api/http"
	"github.com/gsoultan/anubis/internal/platform/config"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/platform/migrate"
	"github.com/gsoultan/anubis/internal/platform/ratelimit"
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
		conn, cerr := pgx.Connect(ctx, cfg.DatabaseURL)
		if cerr != nil {
			return cerr
		}
		_, merr := migrate.NewRunner(migrations.FS, logger).Run(ctx, conn)
		conn.Close(context.WithoutCancel(ctx))
		if merr != nil {
			if errors.Is(merr, migrate.ErrNeedsBaseline) {
				// Common after bench/rebuild.sh: the schema IS at head, only
				// the tracking is empty. Serving is safe; say what to run.
				logger.Warn("schema present but untracked — serving anyway; run `anubisd baseline` to record it")
			} else {
				return merr
			}
		}
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// One pool and one transaction mechanism; each bounded context gets its
	// own repository over its own generated queries.
	db := database.New(pool)
	app, err := newApplication(ctx, cfg, db, logger)
	if err != nil {
		return err
	}
	defer app.close()

	limiter := ratelimit.New()
	opts := connect.WithInterceptors(
		apiconnect.NewMetaInterceptor(),
		apiconnect.NewAuthnInterceptor(cfg.Issuer, app.ring, app.tenancy, app.identity, app.clock),
	)

	rpc := http.NewServeMux()
	app.registerRPC(rpc, opts, limiter, logger)

	srv := apihttp.NewServer(logger, cfg.UIOrigin, rpc, apihttp.NewHealthHandler(pool, app.ring))
	app.registerHTTP(ctx, srv, cfg, limiter, logger)

	httpSrv := srv.HTTPServer(cfg.Listen)

	if cfg.DebugListen != "" {
		dbg := apihttp.DebugServer(cfg.DebugListen)
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

// anubisv1connect is referenced by the RPC registration in application.go;
// keeping the import here would duplicate it.
var _ = anubisv1connect.NewAuthServiceHandler
