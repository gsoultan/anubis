// Package localtoken implements anb.local.v1 — AEAD-sealed internal state
// tokens (MFA challenges, password resets, device enrolment, back-channel
// state). These never cross a trust boundary: only Anubis mints them and only
// Anubis reads them, so symmetric AES-256-GCM with HKDF key derivation is the
// whole design (docs/architecture.md, token table).
//
// Format:
//
//	anb.local.v1.<b64url(kid)>.<b64url(nonce || ciphertext)>
//
// The purpose string is bound as AAD: an MFA token replayed at the password
// reset endpoint fails authentication, not just validation. Single use is the
// caller's job via one_time_tokens keyed on the token's jti.
package localtoken

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	prefix    = "anb.local.v1."
	nonceSize = 12
)

var (
	ErrMalformed = errors.New("localtoken: malformed token")
	ErrPurpose   = errors.New("localtoken: purpose mismatch")
	ErrExpired   = errors.New("localtoken: expired")
	ErrOpen      = errors.New("localtoken: decryption failed")

	b64 = base64.RawURLEncoding.Strict()
)

// deriveKey turns the ring's 32-byte local secret into the AEAD key. The
// constant info string versions the derivation with the format.
func deriveKey(secret []byte) ([]byte, error) {
	return hkdf.Key(sha256.New, secret, nil, "anubis/local/v1", 32)
}

func newAEAD(secret []byte) (cipher.AEAD, error) {
	key, err := deriveKey(secret)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal encrypts data under the keyed purpose with a TTL. jti identifies the
// token for single-use consumption; generate it with crypto/rand.
func Seal(secret []byte, kid, purpose, jti string, data any, ttl time.Duration, now time.Time) (string, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	env, err := json.Marshal(envelope{
		Purpose: purpose,
		Expires: now.Add(ttl).Unix(),
		TokenID: jti,
		Data:    raw,
	})
	if err != nil {
		return "", err
	}
	aead, err := newAEAD(secret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := aead.Seal(nil, nonce, env, []byte(prefix+purpose))

	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteString(b64.EncodeToString([]byte(kid)))
	sb.WriteByte('.')
	sb.WriteString(b64.EncodeToString(append(nonce, ct...)))
	return sb.String(), nil
}

// Kid extracts the key id without opening the token, so the caller can select
// the right secret from the (bounded, in-memory) keyring.
func Kid(token string) (string, error) {
	if !strings.HasPrefix(token, prefix) {
		return "", ErrMalformed
	}
	rest := token[len(prefix):]
	i := strings.IndexByte(rest, '.')
	if i <= 0 {
		return "", ErrMalformed
	}
	kid, err := b64.DecodeString(rest[:i])
	if err != nil {
		return "", ErrMalformed
	}
	return string(kid), nil
}

// Open authenticates, decrypts and validates the token, returning its jti and
// payload bytes.
func Open(secret []byte, token, purpose string, now time.Time) (jti string, data []byte, err error) {
	if !strings.HasPrefix(token, prefix) {
		return "", nil, ErrMalformed
	}
	rest := token[len(prefix):]
	i := strings.IndexByte(rest, '.')
	if i <= 0 || strings.IndexByte(rest[i+1:], '.') >= 0 {
		return "", nil, ErrMalformed
	}
	body, err := b64.DecodeString(rest[i+1:])
	if err != nil || len(body) <= nonceSize {
		return "", nil, ErrMalformed
	}
	aead, err := newAEAD(secret)
	if err != nil {
		return "", nil, err
	}
	plain, err := aead.Open(nil, body[:nonceSize], body[nonceSize:], []byte(prefix+purpose))
	if err != nil {
		return "", nil, ErrOpen
	}
	var env envelope
	if err := json.Unmarshal(plain, &env); err != nil {
		return "", nil, ErrMalformed
	}
	if env.Purpose != purpose {
		return "", nil, ErrPurpose
	}
	if now.Unix() > env.Expires {
		return "", nil, ErrExpired
	}
	return env.TokenID, env.Data, nil
}
