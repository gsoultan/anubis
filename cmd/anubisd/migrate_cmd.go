package main

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/gsoultan/anubis/internal/config"
	"github.com/gsoultan/anubis/internal/migrate"
	"github.com/gsoultan/anubis/migrations"
)

func runMigrate(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer conn.Close(context.WithoutCancel(ctx))

	res, err := migrate.NewRunner(migrations.FS, logger).Run(ctx, conn)
	if err != nil {
		return err
	}
	logger.Info("migrations complete",
		"applied", len(res.Applied), "skipped", res.Skipped, "drifted", len(res.Drifted))
	return nil
}
