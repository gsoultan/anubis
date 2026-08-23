// Package pii seals personal data under a per-identity key so that deleting
// the key makes the data unrecoverable — crypto-shredding (docs/security.md).
//
// Why not just DELETE the rows: an identity's grants, sessions and audit
// entries reference it. Deleting would either cascade (destroying the audit
// trail that proves what access existed) or orphan (breaking referential
// integrity). Shredding keeps the skeleton and destroys the flesh.
//
// The key never exists in the database in the clear: it is sealed with the
// KMS-held master key, exactly like signing keys.
package pii

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// KeySize is the per-identity key length.
const KeySize = 32

var (
	// ErrShredded means the key is gone: the data can never be read again.
	// This is a SUCCESSFUL erasure observed after the fact, not a fault.
	ErrShredded = errors.New("pii: key has been shredded; data is unrecoverable")
	ErrOpen     = errors.New("pii: decryption failed")
)

// NewKey mints key material for one identity.
func NewKey() ([]byte, error) {
	k := make([]byte, KeySize)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	return k, nil
}

func aead(key []byte) (cipher.AEAD, error) {
	dk, err := hkdf.Key(sha256.New, key, nil, "anubis/pii/v1", 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(dk)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal encrypts one field. The field name is bound as additional data, so a
// ciphertext copied from `email` into `phone` fails to open rather than
// silently decoding as the wrong field.
func Seal(key []byte, field string, plaintext []byte) (string, error) {
	a, err := aead(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(
		a.Seal(nonce, nonce, plaintext, []byte(field))), nil
}

// Open reverses Seal. A nil key means the identity was shredded.
func Open(key []byte, field, sealed string) ([]byte, error) {
	if len(key) == 0 {
		return nil, ErrShredded
	}
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		return nil, ErrOpen
	}
	a, err := aead(key)
	if err != nil {
		return nil, err
	}
	if len(raw) < a.NonceSize() {
		return nil, ErrOpen
	}
	out, err := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], []byte(field))
	if err != nil {
		return nil, ErrOpen
	}
	return out, nil
}
