package main

import (
	"context"
	"time"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
)

// storeKeyRotator implements the tenancy admin plane's KeyRotator over the
// auth context's key repository. It lives in the composition root because it
// is the only place holding the process master key.
type storeKeyRotator struct {
	keys   authport.KeyRepository
	master []byte
}

func (r *storeKeyRotator) PrepareKey(ctx context.Context, purpose string) (*authdomain.KeyRecord, error) {
	now := time.Now()
	var k *keyring.Key
	var err error
	if purpose == keyring.PurposeLocal {
		k, err = keyring.GenerateLocalKey(now, keyLifetime)
	} else {
		purpose = keyring.PurposeAccess
		k, err = keyring.GenerateAccessKey(now, keyLifetime)
	}
	if err != nil {
		return nil, err
	}
	material := k.Secret
	if k.Purpose == keyring.PurposeAccess {
		material = k.Private.Seed()
	}
	sealed, err := keyring.SealSecret(r.master, k.Kid, material)
	if err != nil {
		return nil, err
	}
	rec := authdomain.KeyRecord{
		Kid: k.Kid, Alg: k.Alg, Status: keyring.StatusPending, Purpose: purpose,
		PublicKey: orEmptyBytes(k.Public), PrivateKeyEnc: sealed,
		NotBefore: k.NotBefore, NotAfter: k.NotAfter,
	}
	if err := r.keys.CreateKey(ctx, rec); err != nil {
		return nil, err
	}
	return &rec, nil
}
