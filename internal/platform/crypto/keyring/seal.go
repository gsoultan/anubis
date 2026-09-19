package keyring

// Sealing private key material at rest. signing_keys.private_key_enc holds
//
//	nonce(12) || AES-256-GCM(HKDF(master, "anubis/keyseal/v1"), seed|secret)
//
// with the kid as AAD, binding each ciphertext to its row. The master key is
// KMS-held in production and ANUBIS_MASTER_KEY (base64url, 32 bytes) in dev.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

func sealAEAD(master []byte) (cipher.AEAD, error) {
	key, err := hkdf.Key(sha256.New, master, nil, "anubis/keyseal/v1", 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// SealSecret encrypts 32 bytes of key material (an Ed25519 seed or a local
// AEAD secret) for storage.
func SealSecret(master []byte, kid string, material []byte) ([]byte, error) {
	aead, err := sealAEAD(master)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(nonce, aead.Seal(nil, nonce, material, []byte(kid))...), nil
}

// OpenSecret reverses SealSecret.
func OpenSecret(master []byte, kid string, sealed []byte) ([]byte, error) {
	if len(sealed) <= 12 {
		return nil, errors.New("keyring: sealed blob too short")
	}
	aead, err := sealAEAD(master)
	if err != nil {
		return nil, err
	}
	material, err := aead.Open(nil, sealed[:12], sealed[12:], []byte(kid))
	if err != nil {
		return nil, errors.New("keyring: unseal failed (wrong master key?)")
	}
	return material, nil
}

// ErrSealingKeyGone means the key a secret was sealed under is no longer in
// the ring, so the secret can never be read again.
//
// It is a distinct error because it needs a distinct answer. An operator has
// to re-enrol the factor; the holder has done nothing wrong. Folding it into
// "invalid code" would tell the one person who can fix it that somebody
// mistyped six digits.
var ErrSealingKeyGone = errors.New("keyring: the key this secret was sealed under is no longer in the ring")

// OpenNamedSecret opens a secret sealed under a NAMED local key.
//
// A secret written once and read for years cannot be opened with "whichever
// key is active now": rotating the key would make every stored secret
// unreadable, and the rotation is exactly what an incident demands. So the
// caller stores the kid and passes it back here.
//
// An empty kid means the record predates that discipline. Fall back to the
// active local key — which is what the old code did unconditionally, so this
// is no worse — and return the kid that worked so the caller can record it
// and stop guessing.
func OpenNamedSecret(r *Ring, kid, aad string, sealed []byte) (material []byte, usedKid string, err error) {
	if kid == "" {
		k, err := r.ActiveLocal()
		if err != nil {
			return nil, "", err
		}
		m, err := OpenSecret(k.Secret, aad, sealed)
		if err != nil {
			return nil, "", err
		}
		return m, k.Kid, nil
	}
	k, err := r.Lookup(kid)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %s", ErrSealingKeyGone, kid)
	}
	m, err := OpenSecret(k.Secret, aad, sealed)
	if err != nil {
		return nil, "", err
	}
	return m, kid, nil
}
