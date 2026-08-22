package anubis

import (
	"errors"

	"github.com/gsoultan/anubis/pkg/anubis/keys"
)

var (
	ErrExpired      = errors.New("anubis: token expired")
	ErrNotYetValid  = errors.New("anubis: token not yet valid (check NTP)")
	ErrIssuer       = errors.New("anubis: issuer mismatch")
	ErrAudience     = errors.New("anubis: audience mismatch")
	ErrNoAudience   = errors.New("anubis: verifier requires an audience — refusing to skip the aud check")
	ErrTokenVersion = errors.New("anubis: unsupported token version")

	// ErrUnknownKid is re-exported so consumers need not import the keys
	// package to recognise the rejection.
	ErrUnknownKid = keys.ErrUnknownKid
)
