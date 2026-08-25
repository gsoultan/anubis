package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	authpg "github.com/gsoultan/anubis/internal/auth/adapter/postgres"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	"github.com/gsoultan/anubis/internal/platform/config"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// keys init|list|prepare|promote — the pending -> active -> retiring
// lifecycle, plus init for the first key on a fresh installation.
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
	store := authpg.New(database.New(pool))

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
	case "init":
		// A fresh production install has NO signing key: AutoKeys is off
		// outside dev on purpose, so nothing mints one behind your back.
		// That left a new installation unable to issue a single token —
		// /readyz 503, every login failing — with the recovery being two
		// commands nobody had written down. This is those two commands,
		// and it REFUSES once a key exists so it can never silently
		// rotate a live installation.
		records, lerr := store.SigningKeys(ctx)
		if lerr != nil {
			return lerr
		}
		for _, k := range records {
			if k.Purpose == purpose && k.Status == "active" {
				return fmt.Errorf("an active %s key already exists (%s) — "+
					"to rotate, use: anubisd keys prepare %s && anubisd keys promote %s",
					purpose, k.Kid, purpose, purpose)
			}
		}
		if err := prepareKey(ctx, store, cfg.MasterKey, purpose); err != nil {
			return err
		}
		if _, err := store.DemoteActive(ctx, purpose); err != nil {
			return err
		}
		if _, err := store.PromotePending(ctx, purpose); err != nil {
			return err
		}
		logger.Info("signing key created and activated", "purpose", purpose)
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
		return fmt.Errorf("unknown keys subcommand %q (init|list|prepare|promote)", sub)
	}
}

func prepareKey(ctx context.Context, keys authport.KeyRepository, master []byte, purpose string) error {
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
	if err := keys.CreateKey(ctx, authdomain.KeyRecord{
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
