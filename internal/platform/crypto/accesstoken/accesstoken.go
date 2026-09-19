// Package accesstoken reads and verifies an access token in either of the two
// formats Anubis issues, so that no call site has to know there are two.
//
// ADR-0001 chose PASETO v4.public and kept JWS as a hedge for consumers whose
// stack has no PASETO support; applications.token_format picks per
// application. That choice is invisible to a verifier, and it has to be:
// before this package, four call sites each branched on a "v4.public."
// prefix, and enabling JWS for one application would have made three of them
// silently stop recognising its tokens — the gate would deny, introspection
// would report inactive, and revoke would quietly revoke nothing.
//
// The format is chosen by the token's FORMAT MARKER, never by an algorithm
// field inside it. Both codecs are Ed25519 and nothing else, so the choice
// cannot downgrade anything; it selects an encoding, not a cipher.
package accesstoken

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gsoultan/anubis/pkg/anubis/jws"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

// PasetoPrefix is the marker a v4.public token must start with.
const PasetoPrefix = "v4.public."

// ErrMalformed means the token is neither format.
var ErrMalformed = errors.New("accesstoken: not a recognised access token")

// IsAccessToken reports whether a string is shaped like either format. It is
// a cheap discriminator for callers that also accept opaque refresh tokens,
// which carry an "anb_rt_" prefix and no structure.
func IsAccessToken(token string) bool {
	if strings.HasPrefix(token, PasetoPrefix) {
		return true
	}
	_, _, _, err := jws.Parse(token)
	return err == nil
}

// Kid returns the key id the token names, WITHOUT verifying it.
//
// Untrusted: callers may use it only to index a bounded key set they already
// hold. It must never select a key to FETCH.
func Kid(token string) (string, error) {
	if strings.HasPrefix(token, PasetoPrefix) {
		_, _, footer, err := paseto.Parse(token)
		if err != nil {
			return "", err
		}
		if len(footer) == 0 {
			return "", nil
		}
		var tf struct {
			Kid string `json:"kid"`
		}
		if err := json.Unmarshal(footer, &tf); err != nil {
			return "", err
		}
		return tf.Kid, nil
	}
	h, _, _, err := jws.Parse(token)
	if err != nil {
		return "", ErrMalformed
	}
	return h.Kid, nil
}

// ClaimsUnverified returns the claims bytes WITHOUT checking the signature.
//
// There is exactly one legitimate caller: revoke, where a leaked token being
// revoked by whoever found it is the desired outcome. It is named this way so
// that any other use reads as wrong at the call site.
func ClaimsUnverified(token string) ([]byte, error) {
	if strings.HasPrefix(token, PasetoPrefix) {
		msg, _, _, err := paseto.Parse(token)
		return msg, err
	}
	_, payload, _, err := jws.Parse(token)
	if err != nil {
		return nil, ErrMalformed
	}
	return payload, nil
}

// Verify checks the signature under pk and returns the claims bytes.
func Verify(pk ed25519.PublicKey, token string) ([]byte, error) {
	if strings.HasPrefix(token, PasetoPrefix) {
		msg, _, err := paseto.Verify(pk, token, nil)
		return msg, err
	}
	payload, _, err := jws.Verify(pk, token)
	return payload, err
}
