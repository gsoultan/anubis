// Package secret generates and hashes high-entropy opaque credentials
// (refresh tokens, API keys, SSO cookies, nonces).
//
// High-entropy secrets are hashed with plain SHA-256 — a slow KDF is for
// low-entropy passwords and would only add latency here (docs/security.md,
// rule 3). Comparison of any secret goes through Equal, never ==.
package secret

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strings"
)

// New returns n random bytes as unpadded base64url. 32 bytes = 256 bits.
func New(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Hash is the storage form of an opaque secret: sha256 of its text.
func Hash(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

// Equal compares in constant time.
func Equal(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// API keys: anb_live_<prefix>_<secret>. The prefix is stored in clear and
// indexed (credentials.lookup_key = "anb_live_<prefix>") so lookup is one
// probe; the secret part is stored only as its hash.

const apiKeyPrefixLen = 8

// NewAPIKey mints a key and returns (fullKey, lookupKey, secretHash).
func NewAPIKey() (full, lookup string, hash []byte, err error) {
	prefix, err := New(6) // 6 bytes -> 8 chars base64url
	if err != nil {
		return "", "", nil, err
	}
	prefix = prefix[:apiKeyPrefixLen]
	sec, err := New(32)
	if err != nil {
		return "", "", nil, err
	}
	full = "anb_live_" + prefix + "_" + sec
	return full, "anb_live_" + prefix, Hash(sec), nil
}

// SplitAPIKey parses a presented key into its lookup key and secret part.
func SplitAPIKey(key string) (lookup, sec string, ok bool) {
	if !strings.HasPrefix(key, "anb_live_") {
		return "", "", false
	}
	rest := key[len("anb_live_"):]
	i := strings.IndexByte(rest, '_')
	if i != apiKeyPrefixLen || i+1 >= len(rest) {
		return "", "", false
	}
	return "anb_live_" + rest[:i], rest[i+1:], true
}
