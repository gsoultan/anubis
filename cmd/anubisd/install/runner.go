package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	apihttp "github.com/gsoultan/anubis/internal/api/http"
	controlpg "github.com/gsoultan/anubis/internal/control/adapter/postgres"
	controlapp "github.com/gsoultan/anubis/internal/control/app"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	identitypg "github.com/gsoultan/anubis/internal/identity/adapter/postgres"
	"github.com/gsoultan/anubis/internal/platform/config"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/platform/migrate"
	tenancypg "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres"
	"github.com/gsoultan/anubis/migrations"
)

// installer performs a first run. It lives in the composition root because
// building a database from a form submission is exactly the wiring decision
// this file exists to own — no bounded context should be able to do it.
type installer struct {
	listen   string
	issuer   string
	uiOrigin string
	logger   *slog.Logger
}

var _ apihttp.SetupRunner = (*installer)(nil)

// toInput maps the transport's plain form into the domain's input, which is
// where the rules about what a valid installation looks like live.
func (i *installer) toInput(in apihttp.SetupRequest) controldomain.SetupInput {
	return controldomain.SetupInput{
		Token:           in.Token,
		DBHost:          in.DBHost,
		DBPort:          in.DBPort,
		DBName:          in.DBName,
		DBUser:          in.DBUser,
		DBPassword:      in.DBPassword,
		DBSSLMode:       in.DBSSLMode,
		FirstTenantSlug: in.FirstTenantSlug,
		FirstTenantName: in.FirstTenantName,
		OwnerUsername:   in.OwnerUsername,
		OwnerEmail:      in.OwnerEmail,
		OwnerPassword:   in.OwnerPassword,
	}
}

func (i *installer) Validate(in apihttp.SetupRequest) map[string]string {
	return i.toInput(in).Problems()
}

func (i *installer) ValidateDatabase(in apihttp.SetupRequest) map[string]string {
	return i.toInput(in).DatabaseProblems()
}

func (i *installer) fileConfig(in apihttp.SetupRequest) *config.FileConfig {
	ssl := in.DBSSLMode
	if ssl == "" {
		ssl = "require"
	}
	return &config.FileConfig{
		DBHost: in.DBHost, DBPort: in.DBPort, DBName: in.DBName,
		DBUser: in.DBUser, DBPassword: in.DBPassword, DBSSLMode: ssl,
		Listen: i.listen, Issuer: i.issuer, UIOrigin: i.uiOrigin,
	}
}

func (i *installer) TestConnection(ctx context.Context, in apihttp.SetupRequest) error {
	conn, err := pgx.Connect(ctx, i.fileConfig(in).DatabaseURL())
	if err != nil {
		return err
	}
	defer conn.Close(context.WithoutCancel(ctx))
	return conn.Ping(ctx)
}

// Install migrates, provisions, and only then writes the config file.
//
// The order is the safety property. config.yaml existing is what tells this
// server it has an installation, so writing it before the database is ready
// would leave a server that refuses to run its own installer and cannot serve
// either. Written last, a failure anywhere leaves the installer open.
func (i *installer) Install(ctx context.Context, in apihttp.SetupRequest) error {
	fc := i.fileConfig(in)
	dsn := fc.DatabaseURL()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	if _, err := migrate.NewRunner(migrations.FS, i.logger).Run(ctx, conn); err != nil {
		conn.Close(context.WithoutCancel(ctx))
		if !errors.Is(err, migrate.ErrNeedsBaseline) {
			return fmt.Errorf("migrate: %w", err)
		}
		// The schema is already at head with no tracking — adopting an
		// existing database is a legitimate way to install.
		i.logger.Warn("schema present but untracked; adopting it")
	}
	conn.Close(context.WithoutCancel(ctx))

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	db := database.New(pool)

	if err := db.WithinTx(ctx, func(ctx context.Context) error {
		// The owner is a PLATFORM USER, not an identity: whoever runs an
		// installation is not a member of anything it hosts (ADR-0011).
		control := controlpg.New(db)
		if _, oerr := controlapp.CreatePlatformOwner(ctx, control, control,
			in.OwnerUsername, in.OwnerEmail, in.OwnerPassword); oerr != nil {
			return oerr
		}
		// The first tenant, if one was asked for, is an ordinary tenant with
		// an internal realm and NOBODY in it: its people arrive later, and
		// whoever administers it is a platform user with an assignment — a
		// different population from anything created here.
		if in.FirstTenantSlug == "" {
			return nil
		}
		_, perr := controlapp.Provision(ctx, controlapp.ProvisionInput{
			TenantSlug: in.FirstTenantSlug,
			TenantName: in.FirstTenantName,
		}, tenancypg.New(db), identitypg.New(db))
		return perr
	}); err != nil {
		return fmt.Errorf("provision: %w", err)
	}

	key, generated, err := config.EnsureMasterKey()
	if err != nil {
		return fmt.Errorf("master key: %w", err)
	}
	if generated {
		i.logger.Warn("generated a master key — back this file up, the config cannot be read without it",
			"path", config.KeyPath())
	}
	if err := fc.Save(config.ConfigPath(), key); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	i.logger.Info("installation complete", "config", config.ConfigPath(), "first_tenant", in.FirstTenantSlug)
	return nil
}
