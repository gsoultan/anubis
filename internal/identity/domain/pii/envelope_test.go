package pii

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func mustKey(t *testing.T) []byte {
	t.Helper()
	k, err := NewKey()
	if err != nil {
		t.Fatalf("new key: %v", err)
	}
	return k
}

func TestAttributesSurviveARoundTrip(t *testing.T) {
	key := mustKey(t)
	in := map[string]string{"date_of_birth": "1984-02-29", "note": "case 41/b"}
	sealed, err := SealAttributes(key, "id-1", in)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	out, err := OpenAttributes(key, "id-1", sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("want %d attributes, got %d", len(in), len(out))
	}
	for k, v := range in {
		if out[k] != v {
			t.Fatalf("%s: want %q, got %q", k, v, out[k])
		}
	}
}

// The column is what leaks in a database dump, so what it holds is the whole
// point: neither the values nor the field NAMES may be readable in it.
func TestTheStoredColumnRevealsNothing(t *testing.T) {
	key := mustKey(t)
	sealed, err := SealAttributes(key, "id-1",
		map[string]string{"hiv_status": "positive"})
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	for _, secret := range []string{"hiv_status", "positive"} {
		if strings.Contains(string(sealed), secret) {
			t.Fatalf("%q is readable in the stored column: %s", secret, sealed)
		}
	}
	var e envelope
	if err := json.Unmarshal(sealed, &e); err != nil {
		t.Fatalf("stored value is not the documented envelope: %v", err)
	}
	if e.V != EnvelopeVersion || e.Sealed == "" {
		t.Fatalf("envelope is malformed: %+v", e)
	}
}

// The realistic attack on a per-row key is not breaking the cipher, it is
// moving a ciphertext onto a row you are allowed to read. The identity id is
// bound as additional data precisely so that fails.
func TestACiphertextMovedToAnotherIdentityWillNotOpen(t *testing.T) {
	key := mustKey(t)
	sealed, err := SealAttributes(key, "victim", map[string]string{"salary": "180000"})
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := OpenAttributes(key, "attacker", sealed); !errors.Is(err, ErrOpen) {
		t.Fatalf("a row-swapped ciphertext opened, or failed wrongly: %v", err)
	}
}

func TestShreddedKeyReportsErasureNotCorruption(t *testing.T) {
	key := mustKey(t)
	sealed, err := SealAttributes(key, "id-1", map[string]string{"note": "x"})
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := OpenAttributes(nil, "id-1", sealed); !errors.Is(err, ErrShredded) {
		t.Fatalf("want ErrShredded, got %v", err)
	}
}

func TestEmptyAttributesNeedNoKey(t *testing.T) {
	sealed, err := SealAttributes(nil, "id-1", map[string]string{})
	if err != nil {
		t.Fatalf("seal empty: %v", err)
	}
	if string(sealed) != "{}" {
		t.Fatalf("empty attributes stored as %q, want {}", sealed)
	}
	out, err := OpenAttributes(nil, "id-1", sealed)
	if err != nil || len(out) != 0 {
		t.Fatalf("empty round trip: %v %v", out, err)
	}
}

// A row written before this feature holds '{}' or nothing at all. Neither may
// be reported as an error, or every pre-existing identity reads as broken.
func TestLegacyRowsReadAsEmpty(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte(""), []byte("{}")} {
		out, err := OpenAttributes(nil, "id-1", raw)
		if err != nil || len(out) != 0 {
			t.Fatalf("legacy value %q: %v %v", raw, out, err)
		}
	}
}

func TestPlaintextInTheColumnIsRejected(t *testing.T) {
	key := mustKey(t)
	_, err := OpenAttributes(key, "id-1", []byte(`{"employee_id":"E-1"}`))
	if !errors.Is(err, ErrBadEnvelope) {
		t.Fatalf("want ErrBadEnvelope for plaintext, got %v", err)
	}
}
