package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gsoultan/anubis/internal/config"
	"github.com/gsoultan/anubis/internal/crypto/keyring"
	"github.com/gsoultan/anubis/internal/repository"
	"github.com/gsoultan/anubis/internal/repository/postgres"
)

// keys list|prepare|promote — the pending -> active -> retiring lifecycle.
// prepare mints a PENDING key (publish it, warm caches); promote flips
// active -> retiring and pending -> active.
func runKeys(ctx context.Context, logger *slog.Logger, args []string) error {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	purpose := keyring.PurposeAccess
	if len(args) > 1 {
		purpose = args[1]
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
	store := postgres.NewStore(pool)

	switch sub {
	case "list":
		records, err := store.SigningKeys(ctx)
		if err != nil {
			return err
		}
		for _, k := range records {
			fmt.Printf("%-24s %-8s %-9s %-8s nbf=%s naf=%s\n",
				k.Kid, k.Purpose, k.Status, k.Alg,
				k.NotBefore.Format(time.RFC3339), k.NotAfter.Format(time.RFC3339))
		}
		return nil
	case "prepare":
		return prepareKey(ctx, store, cfg.MasterKey, purpose)
	case "promote":
		if _, err := store.DemoteActive(ctx, purpose); err != nil {
			return err
		}
		n, err := store.PromotePending(ctx, purpose)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("no pending %s key to promote — run: anubisd keys prepare %s", purpose, purpose)
		}
		logger.Info("key promoted", "purpose", purpose)
		return nil
	default:
		return fmt.Errorf("unknown keys subcommand %q (list|prepare|promote)", sub)
	}
}

func prepareKey(ctx context.Context, keys repository.KeyRepository, master []byte, purpose string) error {
	now := time.Now()
	var k *keyring.Key
	var err error
	if purpose == keyring.PurposeLocal {
		k, err = keyring.GenerateLocalKey(now, keyLifetime)
	} else {
		k, err = keyring.GenerateAccessKey(now, keyLifetime)
	}
	if err != nil {
		return err
	}
	material := k.Secret
	if k.Purpose == keyring.PurposeAccess {
		material = k.Private.Seed()
	}
	sealed, err := keyring.SealSecret(master, k.Kid, material)
	if err != nil {
		return err
	}
	if err := keys.CreateKey(ctx, repository.KeyRecord{
		Kid: k.Kid, Alg: k.Alg, Status: keyring.StatusPending, Purpose: k.Purpose,
		PublicKey: orEmptyBytes(k.Public), PrivateKeyEnc: sealed,
		NotBefore: k.NotBefore, NotAfter: k.NotAfter,
	}); err != nil {
		return err
	}
	fmt.Printf("pending %s key created: %s (publish, then: anubisd keys promote %s)\n",
		purpose, k.Kid, purpose)
	return nil
}
