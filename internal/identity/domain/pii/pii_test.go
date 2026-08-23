package pii

import (
	"bytes"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	k, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := Seal(k, "email", []byte("applicant@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Open(k, "email", sealed)
	if err != nil || !bytes.Equal(got, []byte("applicant@example.com")) {
		t.Fatalf("round trip: %q %v", got, err)
	}
}

// A ciphertext moved between fields must not open: field names are bound as
// additional data precisely so a copied blob fails loudly.
func TestFieldBinding(t *testing.T) {
	k, _ := NewKey()
	sealed, _ := Seal(k, "email", []byte("secret"))
	if _, err := Open(k, "phone", sealed); err != ErrOpen {
		t.Fatalf("cross-field open: want ErrOpen, got %v", err)
	}
}

func TestWrongKeyAndShredded(t *testing.T) {
	k, _ := NewKey()
	other, _ := NewKey()
	sealed, _ := Seal(k, "email", []byte("secret"))

	if _, err := Open(other, "email", sealed); err != ErrOpen {
		t.Fatalf("wrong key: want ErrOpen, got %v", err)
	}
	// The shredded case is the point of the whole design: no key, no data,
	// forever — and it reports that distinctly from a corrupt ciphertext.
	if _, err := Open(nil, "email", sealed); err != ErrShredded {
		t.Fatalf("shredded: want ErrShredded, got %v", err)
	}
}

func TestTamper(t *testing.T) {
	k, _ := NewKey()
	sealed, _ := Seal(k, "email", []byte("secret"))
	raw := []byte(sealed)
	raw[len(raw)/2] ^= 1
	if _, err := Open(k, "email", string(raw)); err == nil {
		t.Fatal("tampered ciphertext opened")
	}
}
