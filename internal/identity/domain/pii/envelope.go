package pii

import (
	"encoding/json"
	"errors"
)

// EnvelopeVersion is the shape stored in identities.attributes. It exists so a
// future change of cipher or encoding can be told apart from this one instead
// of being guessed at from the ciphertext length.
const EnvelopeVersion = 1

// ErrBadEnvelope means the column holds something that is not a sealed
// attribute envelope. It is deliberately not the same error as a failed
// decryption: one is a shape problem, the other is a key or tampering problem.
var ErrBadEnvelope = errors.New("pii: attributes is not a sealed envelope")

// envelope is what the column holds. The attribute NAMES are inside the
// ciphertext, not beside it: "diagnosis" or "home_address" is itself the
// disclosure, so a shape that listed keys in the clear and sealed only the
// values would leak the thing worth hiding.
type envelope struct {
	V      int    `json:"v"`
	Sealed string `json:"sealed"`
}

// Empty is the value the column holds when an identity has no attributes.
// Storing this rather than an envelope over an empty map means an identity
// that never had attributes needs no key, and reading it needs no decryption.
func Empty() []byte { return []byte(`{}`) }

// IsEmpty reports whether raw is the no-attributes value. A zero-length column
// counts: rows predating this feature are semantically empty.
func IsEmpty(raw []byte) bool {
	if len(raw) == 0 {
		return true
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	return len(m) == 0
}

// aad binds a ciphertext to the row it belongs to. Without it, someone with
// UPDATE on the table could copy one identity's sealed attributes onto another
// identity that shares a key and read them back through the API; with it, the
// copy fails to open.
func aad(identityID string) string { return "attributes:" + identityID }

// SealAttributes encodes attrs into the envelope the column stores. An empty
// map seals to Empty() so that clearing attributes leaves nothing behind.
func SealAttributes(key []byte, identityID string, attrs map[string]string) ([]byte, error) {
	if len(attrs) == 0 {
		return Empty(), nil
	}
	plain, err := json.Marshal(attrs)
	if err != nil {
		return nil, err
	}
	sealed, err := Seal(key, aad(identityID), plain)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{V: EnvelopeVersion, Sealed: sealed})
}

// OpenAttributes reverses SealAttributes. An empty column opens to an empty
// map rather than an error — having no attributes is not a failure. A nil key
// yields ErrShredded, which callers report as erased rather than broken.
func OpenAttributes(key []byte, identityID string, raw []byte) (map[string]string, error) {
	if IsEmpty(raw) {
		return map[string]string{}, nil
	}
	var e envelope
	if err := json.Unmarshal(raw, &e); err != nil || e.Sealed == "" {
		return nil, ErrBadEnvelope
	}
	if e.V != EnvelopeVersion {
		return nil, ErrBadEnvelope
	}
	plain, err := Open(key, aad(identityID), e.Sealed)
	if err != nil {
		return nil, err
	}
	attrs := map[string]string{}
	if err := json.Unmarshal(plain, &attrs); err != nil {
		return nil, ErrOpen
	}
	return attrs, nil
}
