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
