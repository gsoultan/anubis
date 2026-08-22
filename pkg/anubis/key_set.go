package anubis

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// maxKeys bounds the kid map. kid is attacker-controlled input (it arrives in
// tokens); it may only ever index a bounded, pre-loaded, in-memory map —
// never a database query, filesystem path or per-token network fetch.
const maxKeys = 64

// KeySet is an immutable kid -> public key map.
type KeySet struct {
	keys map[string]ed25519.PublicKey
}

func ParseKeysDocument(raw []byte) (*KeySet, error) {
	var doc KeysDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("anubis: keys document: %w", err)
	}
	if len(doc.Keys) > maxKeys {
		return nil, fmt.Errorf("anubis: keys document has %d keys, max %d", len(doc.Keys), maxKeys)
	}
	ks := &KeySet{keys: make(map[string]ed25519.PublicKey, len(doc.Keys))}
	for _, k := range doc.Keys {
		if k.Alg != "Ed25519" {
			continue // pinned algorithm; nothing negotiable
		}
		raw, err := base64.RawURLEncoding.DecodeString(k.PublicKey)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("anubis: key %q: bad public key", k.Kid)
		}
		ks.keys[k.Kid] = ed25519.PublicKey(raw)
	}
	return ks, nil
}

func (ks *KeySet) Get(kid string) (ed25519.PublicKey, bool) {
	pk, ok := ks.keys[kid]
	return pk, ok
}
