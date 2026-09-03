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

	"github.com/gsoultan/storm/runtime/pgxdrv"

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

	// ANUBIS_ENV defaults to dev, and dev substitutes a master key that is a
	// constant in the public source. Every signing key and every PII key
	// sealed under it is readable by anyone with the repository — so an
	// install that meant to be production and forgot one variable is not
	// merely weaker, it is unsealed. The systemd packaging sets ANUBIS_ENV
	// explicitly; a container started with just ANUBIS_DB_URL does not, which
	// is the case this exists for. config.go promised this was said loudly
	// and nothing said it.
	if cfg.InsecureMasterKey {
		logger.Warn("INSECURE: no master key configured, using the built-in DEV key — "+
			"it is a constant in the public source, so nothing sealed under it is confidential. "+
			"Set ANUBIS_ENV=prod (which refuses to boot without a key) and provide "+
			"ANUBIS_MASTER_KEY or ANUBIS_KEY_FILE.",
			"env", cfg.Env, "issuer", cfg.Issuer)
	}

	// Production does NOT migrate on boot, so an operator who skipped that
	// deploy step meets the schema as a raw SQLSTATE from whichever query
	// happens to run first — `relation "signing_keys" does not exist`, which
	// names neither the cause nor the fix. That is the first thing a new
	// container deployment sees now that the image defaults to prod, so it is
	// worth one cheap query to say the useful thing instead.
	if cfg.Env == "prod" {
		conn, cerr := pgx.Connect(ctx, cfg.DatabaseURL)
		if cerr != nil {
			return cerr
		}
		var migrated bool
		qerr := conn.QueryRow(ctx,
			`SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&migrated)
		conn.Close(context.WithoutCancel(ctx))
		if qerr != nil {
			return fmt.Errorf("checking for the schema: %w", qerr)
		}
		if !migrated {
			return errors.New("database has no Anubis schema — run `anubisd migrate` first " +
				"(production does not migrate on boot, so that it stays a deliberate deploy step)")
		}
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

	// Built through storm's constructor, not pgxpool's: the authz context runs
	// storm queries over this pool, and storm installs per-connection codecs
	// (uuid[], Decimal, Interval) that a bare pgxpool has never heard of.
	// Without them a uuid[] parameter takes pgx's generic path and a Decimal
	// fails to encode at all — a trap that waits for the first context to use
	// one. It also refuses an exec mode that would send every value as text,
	// which storm's scanners decode as binary.
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
	// ANUBIS_TRUST_PROXY was parsed into a field nothing read. Setting it
	// configures nothing, and the operator who set it is expecting the
	// opposite — so say so instead of ignoring it.
	if cfg.StrayTrustProxy {
		logger.Warn("ANUBIS_TRUST_PROXY does nothing and has been retired — " +
			"set ANUBIS_TRUSTED_PROXIES to the CIDRs whose X-Forwarded-For you believe. " +
			"Until then the client IP is your proxy's on every request, so per-IP rate " +
			"limits bound the whole installation rather than one caller.")
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
