package keyring

import (
	"crypto/ed25519"
	"time"
)

// Key is one signing or sealing key. Private/Secret are nil on replicas that
// only verify.
type Key struct {
	Kid       string
	Purpose   string
	Alg       string
	Status    string
	Public    ed25519.PublicKey  // purpose=access
	Private   ed25519.PrivateKey // purpose=access, holders only
	Secret    []byte             // purpose=local: 32-byte AEAD input
	NotBefore time.Time
	NotAfter  time.Time
}

// InWindow reports whether now falls inside the key's published validity
// window. A zero bound is unbounded, matching the keys document, where an
// omitted not_before/not_after means "no constraint".
//
// not_after is when verifiers stop *trusting* the key, so it is also the last
// moment it may sign: a token minted just before it is rejected the moment it
// passes.
func (k *Key) InWindow(now time.Time) bool {
	if !k.NotBefore.IsZero() && now.Before(k.NotBefore) {
		return false
	}
	if !k.NotAfter.IsZero() && !now.Before(k.NotAfter) {
		return false
	}
	return true
}
