package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"

	controlpg "github.com/gsoultan/anubis/internal/control/adapter/postgres"
	controlapp "github.com/gsoultan/anubis/internal/control/app"
	identitypg "github.com/gsoultan/anubis/internal/identity/adapter/postgres"
	"github.com/gsoultan/anubis/internal/platform/config"
	"github.com/gsoultan/anubis/internal/platform/database"
	tenancypg "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

// runBootstrap provisions a usable installation: tenant, internal realm,
// admin identity, console + anubis applications, the anubis.admin role
// (pattern anubis:*) and the admin's grant.
func runBootstrap(ctx context.Context, logger *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	tenantSlug := fs.String("tenant", "impack", "tenant slug")
	tenantName := fs.String("name", "Impack", "tenant display name")
	adminUser := fs.String("admin-user", "admin", "admin username")
	adminPass := fs.String("admin-pass", "", "admin password (required)")
	// The platform owner is a different population from the tenant admin
	// above (ADR-0011): they operate the installation rather than belonging
	// to any tenant in it. Setup creates one; this is how a development or
	// scripted install gets one without the wizard.
	platformUser := fs.String("platform-user", "", "platform owner username (optional)")
	platformPass := fs.String("platform-pass", "", "platform owner password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *adminPass == "" {
		return fmt.Errorf("--admin-pass is required")
	}
	if len(*adminPass) < 12 {
		return fmt.Errorf("--admin-pass must be at least 12 characters")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	db := database.New(pool)
	tenancyRepo := tenancypg.New(db)
	identityRepo := identitypg.New(db)

	return db.WithinTx(ctx, func(ctx context.Context) error {
		res, err := controlapp.Provision(ctx, controlapp.ProvisionInput{
			TenantSlug: *tenantSlug,
			TenantName: *tenantName,
			// An ordinary person for exercising tenant-facing flows. NOT an
			// administrator: administration is operator-only (ADR-0011), and
			// the tenant-side admin role no longer exists to grant.
			FirstUsername: *adminUser,
			FirstPassword: *adminPass,
		}, tenancyRepo, identityRepo)
		if err != nil {
			return err
		}
		if *platformUser != "" {
			control := controlpg.New(db)
			id, oerr := controlapp.CreatePlatformOwner(ctx, control, control,
				*platformUser, "", *platformPass)
			if oerr != nil {
				return oerr
			}
			if id != "" {
				logger.Info("platform owner created", "username", *platformUser)
			}
		}
		logger.Info("bootstrap complete", "tenant", *tenantSlug,
			"tenant_created", res.TenantCreated, "first_user_created", res.UserCreated)
		return nil
	})
}
