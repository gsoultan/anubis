// Package legacyhash verifies password hashes that arrived from another
// system and that Anubis will never produce itself.
//
// It exists for one reason: the people moving onto Anubis already have a
// password, and asking every one of them to choose a new one on cut-over day
// is the kind of migration that gets postponed. So the old hash is imported
// as it is, verified ONCE at the person's next sign-in, and replaced on the
// spot by a hash at Anubis's own cost (kdf.Verify reports it as needsRehash
// unconditionally). A legacy hash is a bridge, not a home — the importer
// refuses to store one without an expiry.
//
// This is the ONLY package in the tree allowed to import golang.org/x/crypto,
// and only for bcrypt (ADR-0002, amendment of 2026-10-26; scripts/check/
// deps.sh names the exemption). Verify only: nothing here can MAKE a hash.
package legacyhash

import (
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// ErrNotLegacy says the string is not a format this package recognises.
var ErrNotLegacy = errors.New("legacyhash: not a recognised legacy format")

// AlgBcrypt is what Describe reports for a modular-crypt bcrypt string.
const AlgBcrypt = "bcrypt"

// IsLegacy reports whether encoded LOOKS like a legacy hash. It is a shape
// check only — Verify may still refuse the string as malformed.
func IsLegacy(encoded string) bool {
	return isBcrypt(encoded)
}

// Describe names the algorithm of a legacy hash, or returns ErrNotLegacy.
func Describe(encoded string) (string, error) {
	if isBcrypt(encoded) {
		// Cost and salt are checked by the library on verify; here only the
		// frame is checked, which is what an import needs to refuse a
		// pasted PBKDF2 string or a plain password.
		if _, err := bcrypt.Cost([]byte(encoded)); err != nil {
			return "", ErrNotLegacy
		}
		return AlgBcrypt, nil
	}
	return "", ErrNotLegacy
}

// Verify checks password against a legacy hash. ok is false for a wrong
// password; err is non-nil only when the hash itself is unusable.
func Verify(password, encoded string) (ok bool, err error) {
	if !isBcrypt(encoded) {
		return false, ErrNotLegacy
	}
	err = bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
		return false, nil
	default:
		return false, err
	}
}

// bcrypt's modular-crypt frame: $2$, $2a$, $2b$, $2x$, $2y$ — the variants
// differ only in how a 2011 PHP bug was handled, and the library treats them
// alike. 60 characters is the fixed length of a well-formed hash.
func isBcrypt(encoded string) bool {
	if len(encoded) != 60 || !strings.HasPrefix(encoded, "$2") {
		return false
	}
	switch encoded[2] {
	case '$':
		return true
	case 'a', 'b', 'x', 'y':
		return encoded[3] == '$'
	}
	return false
}
