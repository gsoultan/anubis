// Package kdf hashes passwords with PBKDF2-HMAC-SHA256 from the standard
// library at >= 600k iterations (OWASP-acceptable; ADR-0002 discusses the
// Argon2id trade-off). Algorithm and parameters are stored INSIDE the hash
// string, so upgrading the KDF later is a rehash on next successful login —
// no schema change:
//
//	$pbkdf2-sha256$i=600000$<b64 salt>$<b64 dk>
package kdf

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

const (
	algID             = "pbkdf2-sha256"
	DefaultIterations = 600_000
	saltLen           = 16
	keyLen            = 32
)

var (
	ErrFormat = errors.New("kdf: malformed hash string")
	b64       = base64.RawStdEncoding
)

// Hash derives a fresh salted hash of password at DefaultIterations.
func Hash(password string) (string, error) {
	return hashWith(password, DefaultIterations)
}

func hashWith(password string, iterations int) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk, err := pbkdf2.Key(sha256.New, password, salt, iterations, keyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$%s$i=%d$%s$%s", algID, iterations,
		b64.EncodeToString(salt), b64.EncodeToString(dk)), nil
}

// Verify checks password against encoded. needsRehash reports that the stored
// hash uses weaker parameters than current policy and should be replaced on
// this (successful) login.
func Verify(password, encoded string) (ok bool, needsRehash bool, err error) {
	alg, iterations, salt, want, err := parse(encoded)
	if err != nil {
		return false, false, err
	}
	if alg != algID {
		// Future algorithms parse here. Today anything else fails closed.
		return false, false, ErrFormat
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	if err != nil {
		return false, false, err
	}
	ok = subtle.ConstantTimeCompare(got, want) == 1
	return ok, ok && iterations < DefaultIterations, nil
}

// The uniform-timing defence: when the username does not exist, verify the
// submitted password against this fixed hash so the response time matches the
// real-user path. Otherwise timing is a user-enumeration oracle — invisible
// in functional tests, visible in a histogram (docs/security.md).
var (
	dummyOnce sync.Once
	dummyHash string
)

// Dummy returns a stable, valid hash of an unguessable value.
func Dummy() string {
	dummyOnce.Do(func() {
		var random [24]byte
		if _, err := rand.Read(random[:]); err != nil {
			panic("kdf: crypto/rand unavailable: " + err.Error())
		}
		h, err := Hash(base64.RawStdEncoding.EncodeToString(random[:]))
		if err != nil {
			panic("kdf: dummy hash: " + err.Error())
		}
		dummyHash = h
	})
	return dummyHash
}

func parse(encoded string) (alg string, iterations int, salt, dk []byte, err error) {
	// $pbkdf2-sha256$i=600000$salt$dk
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "" {
		return "", 0, nil, nil, ErrFormat
	}
	alg = parts[1]
	if !strings.HasPrefix(parts[2], "i=") {
		return "", 0, nil, nil, ErrFormat
	}
	iterations, err = strconv.Atoi(parts[2][2:])
	if err != nil || iterations < 1 || iterations > 10_000_000 {
		return "", 0, nil, nil, ErrFormat
	}
	if salt, err = b64.DecodeString(parts[3]); err != nil {
		return "", 0, nil, nil, ErrFormat
	}
	if dk, err = b64.DecodeString(parts[4]); err != nil || len(dk) == 0 || len(dk) > 64 {
		return "", 0, nil, nil, ErrFormat
	}
	return alg, iterations, salt, dk, nil
}
