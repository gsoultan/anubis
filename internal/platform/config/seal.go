package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// The database password in the config file is sealed the same way private
// keys are (internal/platform/crypto/keyring/seal.go):
//
//	enc:v1:base64url( nonce(12) || AES-256-GCM(HKDF(master, label), password) )
//
// The HKDF label differs from the keyring's on purpose. Domain separation
// means a config ciphertext can never be substituted for a sealed signing
// key, or the reverse, even though both are opened with the same master key.
//
// Be clear about what this buys: it stops a password being read over a
// shoulder, pasted into a ticket, or committed to git along with the config.
// It does NOT defend against someone who can read both the config and the
// key — if the master key is a file beside it, that is one filesystem read.
const (
	sealLabel  = "anubis/configseal/v1"
	sealPrefix = "enc:v1:"
)

// ErrNotSealed is returned when a value that should be sealed is plaintext.
var ErrNotSealed = errors.New("config: value is not sealed")

func sealAEAD(master []byte) (cipher.AEAD, error) {
	key, err := hkdf.Key(sha256.New, master, nil, sealLabel, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal encrypts a config secret under the master key.
func Seal(master []byte, plaintext string) (string, error) {
	aead, err := sealAEAD(master)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	blob := append(nonce, aead.Seal(nil, nonce, []byte(plaintext), nil)...)
	return sealPrefix + base64.RawURLEncoding.EncodeToString(blob), nil
}

// Open reverses Seal. A value without the prefix is reported as unsealed
// rather than silently returned: a config that quietly accepted a plaintext
// password would make the encryption optional in practice, and nobody would
// notice which of their installations had it.
func Open(master []byte, value string) (string, error) {
	if !IsSealed(value) {
		return "", ErrNotSealed
	}
	blob, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, sealPrefix))
	if err != nil {
		return "", errors.New("config: sealed value is not valid base64")
	}
	aead, err := sealAEAD(master)
	if err != nil {
		return "", err
	}
	if len(blob) <= aead.NonceSize() {
		return "", errors.New("config: sealed value too short")
	}
	out, err := aead.Open(nil, blob[:aead.NonceSize()], blob[aead.NonceSize():], nil)
	if err != nil {
		return "", errors.New("config: could not open the sealed value (wrong master key?)")
	}
	return string(out), nil
}

// IsSealed reports whether a config value carries the sealed prefix.
func IsSealed(value string) bool { return strings.HasPrefix(value, sealPrefix) }
