package keyload

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"time"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
)

// Lifetime is how long a freshly minted key is valid for. Shared with the
// rotation job and the `anubisd keys` command, so a key made on a schedule
// and one made by hand agree.
const Lifetime = 90 * 24 * time.Hour

// Load reads signing keys from the database, unseals private material
// with the master key, and optionally provisions first keys (dev).
func Load(ctx context.Context, logger *slog.Logger, keys authport.KeyRepository, master []byte, autoProvision bool) (*keyring.Manager, error) {
	ring, err := buildRing(ctx, logger, keys, master)
	if err != nil {
		return nil, err
	}
	if _, aerr := ring.ActiveAccessPresent(); aerr != nil && autoProvision {
		logger.Warn("no active signing keys — provisioning (dev auto-keys)")
		if err := provisionKeys(ctx, keys, master); err != nil {
			return nil, err
		}
		if ring, err = buildRing(ctx, logger, keys, master); err != nil {
			return nil, err
		}
	}
	return keyring.NewManager(ring), nil
}

func buildRing(ctx context.Context, logger *slog.Logger, keys authport.KeyRepository, master []byte) (*keyring.Ring, error) {
	records, err := keys.VerificationKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*keyring.Key, 0, len(records))
	for _, r := range records {
		k := &keyring.Key{
			Kid: r.Kid, Purpose: r.Purpose, Alg: r.Alg, Status: r.Status,
			NotBefore: r.NotBefore, NotAfter: r.NotAfter,
		}
		material, err := keyring.OpenSecret(master, r.Kid, r.PrivateKeyEnc)
		if err != nil {
			// A key that must still SIGN is fatal. Without its private half
			// this instance cannot issue anything, and booting anyway would
			// serve an error on every login instead of saying why once.
			//
			// A RETIRING key is a different case and used to be treated the
			// same. It is loaded to keep verifying tokens signed before the
			// rotation, and verification uses the public half, which is
			// stored in plaintext beside it — NewRing only ever selects an
			// active key to sign with, so a retiring one cannot be picked by
			// accident. Refusing to boot over it turns a botched re-seal into
			// a total outage: observed here when three keys left behind by
			// `anubisd keys reseal` stopped the server starting at all, and
			// the only thing genuinely lost was the ability to sign with keys
			// that must not sign.
			if r.Status != keyring.StatusRetiring {
				return nil, fmt.Errorf("unseal key %s: %w", r.Kid, err)
			}
			logger.Error("retiring key could not be unsealed — it will keep "+
				"verifying, but anything sealed under it can no longer be opened",
				"kid", r.Kid, "purpose", r.Purpose, "error", err)
			if r.Purpose == keyring.PurposeAccess {
				k.Public = ed25519.PublicKey(r.PublicKey)
			}
			out = append(out, k)
			continue
		}
		switch r.Purpose {
		case keyring.PurposeAccess:
			k.Private = ed25519.NewKeyFromSeed(material)
			k.Public = ed25519.PublicKey(r.PublicKey)
		case keyring.PurposeLocal:
			k.Secret = material
		}
		out = append(out, k)
	}
	return keyring.NewRing(out)
}

func provisionKeys(ctx context.Context, keys authport.KeyRepository, master []byte) error {
	now := time.Now()
	access, err := keyring.GenerateAccessKey(now, Lifetime)
	if err != nil {
		return err
	}
	local, err := keyring.GenerateLocalKey(now, Lifetime)
	if err != nil {
		return err
	}
	for _, k := range []*keyring.Key{access, local} {
		material := k.Secret
		if k.Purpose == keyring.PurposeAccess {
			material = k.Private.Seed()
		}
		sealed, err := keyring.SealSecret(master, k.Kid, material)
		if err != nil {
			return err
		}
		if err := keys.CreateKey(ctx, authdomain.KeyRecord{
			Kid: k.Kid, Alg: k.Alg, Status: keyring.StatusActive, Purpose: k.Purpose,
			PublicKey: OrEmptyBytes(k.Public), PrivateKeyEnc: sealed,
			NotBefore: k.NotBefore, NotAfter: k.NotAfter,
		}); err != nil {
			return err
		}
	}
	return nil
}

// OrEmptyBytes maps a nil public key (local-purpose secrets have none) to an
// empty bytea rather than NULL.
func OrEmptyBytes(b []byte) []byte {
	if b == nil {
		return []byte{}
	}
	return b
}
