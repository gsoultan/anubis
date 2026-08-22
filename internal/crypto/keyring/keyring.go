// Package keyring holds signing keys in memory with the rotation lifecycle
// pending -> active -> retiring -> retired (docs/architecture.md).
//
// The cardinal rule: kid lookup is a probe into a BOUNDED, PRE-LOADED,
// in-memory map. Never a database query, never a filesystem path, never a
// network fetch. Attacker-controlled input must not drive I/O.
package keyring

import "errors"

const (
	PurposeAccess = "access" // Ed25519, signs PASETO access tokens
	PurposeLocal  = "local"  // 32-byte secret for anb.local.v1 AEAD

	StatusPending  = "pending"
	StatusActive   = "active"
	StatusRetiring = "retiring"
	StatusRetired  = "retired"

	// maxKeys bounds the map; kid arrives inside attacker-supplied tokens.
	maxKeys = 64
)

var (
	ErrNoActiveKey = errors.New("keyring: no active key for purpose")
	ErrUnknownKid  = errors.New("keyring: unknown kid")
	ErrTooManyKeys = errors.New("keyring: too many keys")
)
