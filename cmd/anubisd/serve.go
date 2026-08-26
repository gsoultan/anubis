package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gsoultan/raorm/runtime/pgxdrv"

	"github.com/gsoultan/anubis/cmd/anubisd/install"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	apihttp "github.com/gsoultan/anubis/internal/api/http"
	"github.com/gsoultan/anubis/internal/platform/config"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/platform/metrics"
	"github.com/gsoultan/anubis/internal/platform/migrate"
	"github.com/gsoultan/anubis/internal/platform/ratelimit"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/migrations"
)

func runServe(ctx context.Context, logger *slog.Logger) error {
	// A first run has no database to ask, so the installer runs first and
	// this function only continues once it has written a config.
	if !config.Configured() {
		if err := install.Run(ctx, logger); err != nil {
			return err
		}
	}

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

	// Pool sized to the DATABASE's capacity, with server-side guards baked
	// into the URL so no caller can forget them.
	poolCfg, err := pgxpool.ParseConfig(cfg.PoolURL())
	if err != nil {
		return err
	}
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnLifetimeJitter = 5 * time.Minute // avoid synchronised reconnect storms
	poolCfg.MaxConnIdleTime = 15 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second

	// Built through raorm's constructor, not pgxpool's: the authz context runs
	// raorm queries over this pool, and raorm installs per-connection codecs
	// (uuid[], Decimal, Interval) that a bare pgxpool has never heard of.
	// Without them a uuid[] parameter takes pgx's generic path and a Decimal
	// fails to encode at all — a trap that waits for the first context to use
	// one. It also refuses an exec mode that would send every value as text,
	// which raorm's scanners decode as binary.
	pool, err := pgxdrv.NewPoolConfig(ctx, poolCfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("database unreachable: %w", err)
	}

	// One pool and one transaction mechanism; each bounded context gets its
	// own repository over its own generated queries.
	db := database.New(pool)
	app, err := newApplication(ctx, cfg, db, logger)
	if err != nil {
		return err
	}
	defer app.close()

	// Behind TLS termination the peer address is always the proxy, so per-IP
	// limits would bound the whole installation rather than one caller.
	// Naming the proxies is what makes the client's own address visible.
	trust, err := authctx.NewProxyTrust(cfg.TrustedProxies)
	if err != nil {
		return fmt.Errorf("ANUBIS_TRUSTED_PROXIES: %w", err)
	}
	if cfg.TrustedProxies != "" {
		logger.Info("trusting X-Forwarded-For from proxies", "cidrs", cfg.TrustedProxies)
	}

	limiter := ratelimit.New()
	opts := connect.WithInterceptors(
		apiconnect.NewMetaInterceptor(trust),
		apiconnect.NewAuthnInterceptor(cfg.Issuer, app.ring, app.tenancy, app.auth, app.control, app.clock),
	)

	rpc := http.NewServeMux()
	app.registerRPC(rpc, opts, limiter, logger)

	metrics.SetBuildInfo(version)
	metrics.RegisterPoolStats(func() metrics.PoolStats {
		s := pool.Stat()
		return metrics.PoolStats{
			Acquired: int64(s.AcquiredConns()), Idle: int64(s.IdleConns()),
			Total: int64(s.TotalConns()), Max: int64(s.MaxConns()),
			EmptyAcquireCount: s.EmptyAcquireCount(),
		}
	})

	health := apihttp.NewHealthHandler(pool, app.ring)
	srv := apihttp.NewServer(logger, cfg.UIOrigin, rpc, health)
	app.registerHTTP(ctx, srv, cfg, health, limiter, logger)
	app.runMaintenance(ctx, db, logger)

	httpSrv := srv.HTTPServer(cfg.Listen, apihttp.Limits{
		RequestTimeout:  cfg.RequestTimeout,
		MaxRequestBytes: cfg.MaxRequestBytes,
	})

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
		// Shutdown order matters: stop accepting first, let in-flight
		// requests finish, and only then close the audit queue — an audit
		// event dropped during drain is a security record lost.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
		defer cancel()
		logger.Info("draining", "grace", cfg.ShutdownGrace.String())
		err := httpSrv.Shutdown(shutdownCtx)
		if err != nil {
			logger.Warn("drain incomplete; forcing close", "error", err)
			_ = httpSrv.Close()
		}
		return err
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
