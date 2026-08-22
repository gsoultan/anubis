package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"time"

	"github.com/gsoultan/anubis/internal/crypto/keyring"
	"github.com/gsoultan/anubis/internal/repository"
)

const keyLifetime = 90 * 24 * time.Hour

// loadRing loads signing keys from the database, unseals private material
// with the master key, and optionally provisions first keys (dev).
func loadRing(ctx context.Context, logger *slog.Logger, keys repository.KeyRepository, master []byte, autoProvision bool) (*keyring.Manager, error) {
	ring, err := buildRing(ctx, keys, master)
	if err != nil {
		return nil, err
	}
	if _, aerr := ring.ActiveAccess(); aerr != nil && autoProvision {
		logger.Warn("no active signing keys — provisioning (dev auto-keys)")
		if err := provisionKeys(ctx, keys, master); err != nil {
			return nil, err
		}
		if ring, err = buildRing(ctx, keys, master); err != nil {
			return nil, err
		}
	}
	return keyring.NewManager(ring), nil
}

func buildRing(ctx context.Context, keys repository.KeyRepository, master []byte) (*keyring.Ring, error) {
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
			return nil, fmt.Errorf("unseal key %s: %w", r.Kid, err)
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

func provisionKeys(ctx context.Context, keys repository.KeyRepository, master []byte) error {
	now := time.Now()
	access, err := keyring.GenerateAccessKey(now, keyLifetime)
	if err != nil {
		return err
	}
	local, err := keyring.GenerateLocalKey(now, keyLifetime)
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
		if err := keys.CreateKey(ctx, repository.KeyRecord{
			Kid: k.Kid, Alg: k.Alg, Status: keyring.StatusActive, Purpose: k.Purpose,
			PublicKey: orEmptyBytes(k.Public), PrivateKeyEnc: sealed,
			NotBefore: k.NotBefore, NotAfter: k.NotAfter,
		}); err != nil {
			return err
		}
	}
	return nil
}

// orEmptyBytes maps a nil public key (local-purpose secrets have none) to an
// empty bytea rather than NULL.
func orEmptyBytes(b []byte) []byte {
	if b == nil {
		return []byte{}
	}
	return b
}
