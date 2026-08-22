package keyring

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"time"
)

// GenerateAccessKey creates a fresh Ed25519 keypair with a random kid.
func GenerateAccessKey(now time.Time, lifetime time.Duration) (*Key, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	kid, err := randomKid("ak")
	if err != nil {
		return nil, err
	}
	return &Key{
		Kid: kid, Purpose: PurposeAccess, Alg: "Ed25519", Status: StatusPending,
		Public: pub, Private: priv,
		NotBefore: now, NotAfter: now.Add(lifetime),
	}, nil
}

// GenerateLocalKey creates a fresh 32-byte AEAD secret.
func GenerateLocalKey(now time.Time, lifetime time.Duration) (*Key, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	kid, err := randomKid("lk")
	if err != nil {
		return nil, err
	}
	return &Key{
		Kid: kid, Purpose: PurposeLocal, Alg: "Ed25519", Status: StatusPending,
		Secret:    secret,
		NotBefore: now, NotAfter: now.Add(lifetime),
	}, nil
}

func randomKid(prefix string) (string, error) {
	var b [9]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(b[:]), nil
}
