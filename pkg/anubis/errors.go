package anubis

import "errors"

var (
	ErrExpired      = errors.New("anubis: token expired")
	ErrNotYetValid  = errors.New("anubis: token not yet valid (check NTP)")
	ErrIssuer       = errors.New("anubis: issuer mismatch")
	ErrAudience     = errors.New("anubis: audience mismatch")
	ErrNoAudience   = errors.New("anubis: verifier requires an audience — refusing to skip the aud check")
	ErrUnknownKid   = errors.New("anubis: unknown kid")
	ErrTokenVersion = errors.New("anubis: unsupported token version")
)
