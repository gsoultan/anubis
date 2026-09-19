// Package jws signs and verifies compact JWS with Ed25519 and nothing else.
//
// It exists for consumers whose stack has no PASETO support — ADR-0001 chose
// v4.public and kept this as the hedge, dormant behind a per-application flag
// (applications.token_format).
//
// What makes JOSE dangerous is negotiation: a verifier that reads `alg` from
// the token it is checking can be told `none`, or told to treat an RSA public
// key as an HMAC secret. This package never reads `alg` to decide anything.
// EdDSA is compiled in; a header naming anything else is rejected before a
// byte is verified, exactly as PASETO's version header works. There is no
// key-discovery here either: the caller supplies the key, and `kid` is a hint
// for choosing it, never an instruction to fetch one.
//
// Standard library only: crypto/ed25519, encoding/base64, encoding/json.
package jws

import (
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

var (
	ErrKeySize      = errors.New("jws: wrong key size")
	ErrMalformed    = errors.New("jws: malformed token")
	ErrBadSig       = errors.New("jws: signature does not verify")
	ErrWrongAlg     = errors.New("jws: header does not say EdDSA")
	ErrWrongType    = errors.New("jws: header typ is not JWT")
	ErrCritHeader   = errors.New("jws: header carries crit, which this verifier does not implement")
	ErrHeaderTooBig = errors.New("jws: header is implausibly large")
)

// Format is the value applications.token_format carries for this codec.
const Format = "jws.eddsa"

// b64 is base64url WITHOUT padding, which is what RFC 7515 §2 requires.
var b64 = base64.RawURLEncoding

// maxHeaderBytes bounds the protected header before it is unmarshalled. A
// token is attacker-supplied; nothing legitimate needs more than this, and
// the bound means a hostile header cannot make the parser do arbitrary work.
const maxHeaderBytes = 1024

// Header is the protected header this codec writes and the only shape it
// accepts. The fields are fixed, not negotiated.
type Header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid,omitempty"`
	// Crit is decoded ONLY so it can be rejected. RFC 7515 §4.1.11 says a
	// verifier that does not understand every extension named here must fail;
	// silently ignoring it is how "critical" becomes advisory.
	Crit []string `json:"crit,omitempty"`
}

// Sign produces a compact JWS over payload, with kid in the protected header.
//
// kid is protected, not a free-floating hint: it is inside the signed bytes,
// so an attacker cannot re-point a valid token at a different key.
func Sign(sk ed25519.PrivateKey, payload []byte, kid string) (string, error) {
	if len(sk) != ed25519.PrivateKeySize {
		return "", ErrKeySize
	}
	h, err := json.Marshal(Header{Alg: "EdDSA", Typ: "JWT", Kid: kid})
	if err != nil {
		return "", err
	}
	signing := b64.EncodeToString(h) + "." + b64.EncodeToString(payload)
	sig := ed25519.Sign(sk, []byte(signing))
	return signing + "." + b64.EncodeToString(sig), nil
}

// Parse splits a token and returns its header and payload WITHOUT verifying
// the signature. Callers must treat both as untrusted until Verify succeeds;
// it exists so the kid can be read to select a verification key.
//
// The algorithm check happens here rather than in Verify so that no caller
// can reach a payload from a token claiming `none` even by mistake.
func Parse(token string) (h Header, payload, sig []byte, err error) {
	// Exactly two dots. A compact JWE has four, and accepting one here would
	// hand a caller the encrypted payload as though it were signed claims.
	first := strings.IndexByte(token, '.')
	if first < 0 {
		return h, nil, nil, ErrMalformed
	}
	rest := token[first+1:]
	second := strings.IndexByte(rest, '.')
	if second < 0 || strings.IndexByte(rest[second+1:], '.') >= 0 {
		return h, nil, nil, ErrMalformed
	}
	headPart := token[:first]
	payPart := rest[:second]
	sigPart := rest[second+1:]

	if len(headPart) > maxHeaderBytes {
		return h, nil, nil, ErrHeaderTooBig
	}
	rawHead, err := b64.DecodeString(headPart)
	if err != nil {
		return h, nil, nil, ErrMalformed
	}
	if err := json.Unmarshal(rawHead, &h); err != nil {
		return h, nil, nil, ErrMalformed
	}
	// The whole point of the package. Compared in constant time for
	// consistency with the rest of the verifier, not because the algorithm
	// name is a secret.
	if subtle.ConstantTimeCompare([]byte(h.Alg), []byte("EdDSA")) != 1 {
		return h, nil, nil, ErrWrongAlg
	}
	if h.Typ != "" && h.Typ != "JWT" {
		return h, nil, nil, ErrWrongType
	}
	if len(h.Crit) > 0 {
		return h, nil, nil, ErrCritHeader
	}
	if payload, err = b64.DecodeString(payPart); err != nil {
		return h, nil, nil, ErrMalformed
	}
	if sig, err = b64.DecodeString(sigPart); err != nil {
		return h, nil, nil, ErrMalformed
	}
	if len(sig) != ed25519.SignatureSize {
		return h, nil, nil, ErrMalformed
	}
	return h, payload, sig, nil
}

// Verify checks the token against pk and returns the payload on success.
func Verify(pk ed25519.PublicKey, token string) (payload []byte, h Header, err error) {
	if len(pk) != ed25519.PublicKeySize {
		return nil, h, ErrKeySize
	}
	h, payload, sig, err := Parse(token)
	if err != nil {
		return nil, h, err
	}
	// Signed bytes are the ORIGINAL encodings, not re-encodings of what was
	// decoded: base64 has non-canonical forms, and re-encoding would verify a
	// string the signer never signed.
	i := strings.LastIndexByte(token, '.')
	if !ed25519.Verify(pk, []byte(token[:i]), sig) {
		return nil, h, ErrBadSig
	}
	return payload, h, nil
}
