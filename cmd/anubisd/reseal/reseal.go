// Package reseal rewraps every secret the master key holds under a new one.
//
// Its own package because cmd/anubisd was at the file limit and this is a
// self-contained procedure: it reads its own configuration, opens its own
// pool, and touches three contexts that nothing else in the command needs
// together.
package reseal

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"

	authpg "github.com/gsoultan/anubis/internal/auth/adapter/postgres"
	controlpg "github.com/gsoultan/anubis/internal/control/adapter/postgres"
	identitypg "github.com/gsoultan/anubis/internal/identity/adapter/postgres"
	"github.com/gsoultan/anubis/internal/platform/config"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The three bindings. Each ciphertext the master key produces is tied to a
// piece of additional authenticated data, and rewrapping under the wrong one
// yields a blob that decrypts to nothing — silently, for every secret in the
// installation, with nothing wrong-looking until somebody tries to sign in.
//
// They are named here so the reseal sweep and the tests that prove it cannot
// drift from each other: a test that supplies its own AAD proves only that it
// agrees with itself.
func signingKeyAAD(kid string) string      { return kid }
func piiKeyAAD(identityID string) string   { return "pii:" + identityID }
func operatorTOTPAAD(userID string) string { return userID }

// Run rewraps every secret the master key holds under a NEW master.
//
// docs/operations.md has required this since it was written — step 4 of the
// signing-key compromise runbook is "rotate the master key and re-seal" — and
// until now nothing could do it. A runbook step with no implementation is
// discovered during the incident it exists for.
//
// The master seals three things, each bound to a different piece of
// additional authenticated data, and getting one wrong produces ciphertext
// that decrypts to nothing:
//
//	signing_keys.private_key_enc     signingKeyAAD
//	pii_keys.key_enc                 piiKeyAAD
//	platform_users.totp_secret_enc   operatorTOTPAAD
//
// It is safe to re-run. Every row is opened with the OLD master first, so a
// row already rewrapped fails that step and is reported rather than
// double-sealed — which is what makes a partial run recoverable.
func Run(ctx context.Context, logger *slog.Logger, args []string) error {
	dry := false
	for _, a := range args {
		switch a {
		case "-n", "--dry-run":
			dry = true
		default:
			return fmt.Errorf("unknown flag %q (usage: anubisd keys reseal [--dry-run])", a)
		}
	}

	old, err := config.MasterKey()
	if err != nil {
		return fmt.Errorf("read the current master key: %w", err)
	}
	raw := os.Getenv("ANUBIS_NEW_MASTER_KEY")
	if raw == "" {
		return errors.New("set ANUBIS_NEW_MASTER_KEY (base64url, 32 bytes) to the master you are rotating TO")
	}
	next, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(next) != 32 {
		return errors.New("ANUBIS_NEW_MASTER_KEY must be 32 bytes, base64url")
	}
	// Rewrapping under the key it is already sealed with would report success
	// while changing nothing, and leave the operator believing a rotation
	// happened.
	if string(old) == string(next) {
		return errors.New("ANUBIS_NEW_MASTER_KEY is the key already in use — nothing would change")
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

	rewrap := func(kind, name string, sealed []byte, aad string) ([]byte, error) {
		material, err := keyring.OpenSecret(old, aad, sealed)
		if err != nil {
			return nil, fmt.Errorf("%s %s did not open with the current master key "+
				"(already rewrapped, or the wrong key is configured): %w", kind, name, err)
		}
		return keyring.SealSecret(next, aad, material)
	}

	var keys, piis, totps int

	// One transaction. A master-key rotation that half happened leaves an
	// installation that cannot start under either key.
	err = db.WithinTx(ctx, func(ctx context.Context) error {
		keys, piis, totps = 0, 0, 0
		return resealAll(ctx, db, dry, rewrap, &keys, &piis, &totps)
	})
	if err != nil {
		return err
	}

	if dry {
		logger.Info("reseal dry run: every secret opened with the current master key",
			"signing_keys", keys, "pii_keys", piis, "operator_totp", totps)
		return nil
	}
	logger.Info("resealed under the new master key",
		"signing_keys", keys, "pii_keys", piis, "operator_totp", totps)
	logger.Warn("the new key is not yet configured — set ANUBIS_MASTER_KEY " +
		"(or ANUBIS_KEY_FILE) to it before restarting, or nothing will unseal")
	return nil
}

// resealAll rewraps every secret the master holds. Split out so the whole
// sweep runs inside one transaction with one error path.
func resealAll(
	ctx context.Context,
	db *database.DB,
	dry bool,
	rewrap func(kind, name string, sealed []byte, aad string) ([]byte, error),
	keys, piis, totps *int,
) error {
	keyStore := authpg.New(db)
	kk, err := keyStore.ResealableKeys(ctx)
	if err != nil {
		return err
	}
	for _, k := range kk {
		sealed, err := rewrap("signing key", k.Kid, k.Sealed, signingKeyAAD(k.Kid))
		if err != nil {
			return err
		}
		if !dry {
			if err := keyStore.ResealKey(ctx, k.ID, sealed); err != nil {
				return err
			}
		}
		*keys++
	}

	idStore := identitypg.New(db)
	pk, err := idStore.ResealablePIIKeys(ctx)
	if err != nil {
		return err
	}
	for _, p := range pk {
		sealed, err := rewrap("pii key", p.KeyID, p.Sealed, piiKeyAAD(p.IdentityID))
		if err != nil {
			return err
		}
		if !dry {
			if err := idStore.ResealPIIKey(ctx, p.KeyID, sealed, ""); err != nil {
				return err
			}
		}
		*piis++
	}

	ctlStore := controlpg.New(db)
	tt, err := ctlStore.ResealableTOTPSecrets(ctx)
	if err != nil {
		return err
	}
	for _, t := range tt {
		sealed, err := rewrap("operator totp", t.IDText, t.Sealed, operatorTOTPAAD(t.IDText))
		if err != nil {
			return err
		}
		if !dry {
			if err := ctlStore.ResealTOTPSecret(ctx, t.ID, sealed); err != nil {
				return err
			}
		}
		*totps++
	}
	return nil
}
