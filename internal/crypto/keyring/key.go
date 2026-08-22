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
