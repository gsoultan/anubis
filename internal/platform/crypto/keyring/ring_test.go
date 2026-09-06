package keyring

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

func accessKey(t *testing.T, kid string, notBefore, notAfter time.Time) *Key {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &Key{
		Kid: kid, Purpose: PurposeAccess, Alg: "Ed25519", Status: StatusActive,
		Public: pub, Private: priv, NotBefore: notBefore, NotAfter: notAfter,
	}
}

func ring(t *testing.T, keys ...*Key) *Ring {
	t.Helper()
	r, err := NewRing(keys)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Signing with a key past its published not_after mints tokens every
// conforming verifier is obliged to reject. The failure has to land here, at
// issuance, and not silently on whatever day the window closed.
func TestActiveAccessRefusesAKeyPastItsWindow(t *testing.T) {
	now := time.Now()
	r := ring(t, accessKey(t, "ak_1", now.Add(-90*24*time.Hour), now.Add(-time.Minute)))

	_, err := r.ActiveAccessAt(now)
	if !errors.Is(err, ErrKeyOutOfWindow) {
		t.Fatalf("signed with an expired key: err = %v, want ErrKeyOutOfWindow", err)
	}
}

func TestActiveAccessRefusesAKeyNotYetValid(t *testing.T) {
	now := time.Now()
	r := ring(t, accessKey(t, "ak_1", now.Add(time.Hour), now.Add(90*24*time.Hour)))

	if _, err := r.ActiveAccessAt(now); !errors.Is(err, ErrKeyOutOfWindow) {
		t.Fatalf("signed with a key that is not yet valid: err = %v", err)
	}
}

func TestActiveAccessAcceptsAKeyInsideItsWindow(t *testing.T) {
	now := time.Now()
	r := ring(t, accessKey(t, "ak_1", now.Add(-time.Hour), now.Add(90*24*time.Hour)))

	k, err := r.ActiveAccessAt(now)
	if err != nil {
		t.Fatalf("refused a valid key: %v", err)
	}
	if k.Kid != "ak_1" {
		t.Fatalf("kid = %q", k.Kid)
	}
}

func TestZeroBoundsAreUnbounded(t *testing.T) {
	r := ring(t, accessKey(t, "ak_1", time.Time{}, time.Time{}))

	if _, err := r.ActiveAccessAt(time.Now()); err != nil {
		t.Fatalf("a key with no published window must be usable: %v", err)
	}
}

func TestNotAfterIsExclusive(t *testing.T) {
	now := time.Now()
	r := ring(t, accessKey(t, "ak_1", now.Add(-time.Hour), now))

	if _, err := r.ActiveAccessAt(now.Add(-time.Second)); err != nil {
		t.Fatalf("one second before not_after: %v", err)
	}
	if _, err := r.ActiveAccessAt(now); !errors.Is(err, ErrKeyOutOfWindow) {
		t.Fatal("trust must stop at not_after, not after it")
	}
}

// Dev auto-provisioning and the operator dashboard ask "is a key configured?".
// If an expired key read as absent, startup would mint a fresh signing key
// unattended — a silent rotation — and the dashboard would hide the key
// exactly when its rotation signal matters most.
func TestActiveAccessPresentStillSeesAnExpiredKey(t *testing.T) {
	now := time.Now()
	r := ring(t, accessKey(t, "ak_1", now.Add(-90*24*time.Hour), now.Add(-time.Minute)))

	k, err := r.ActiveAccessPresent()
	if err != nil {
		t.Fatalf("presence check must ignore the window: %v", err)
	}
	if k.Kid != "ak_1" {
		t.Fatalf("kid = %q", k.Kid)
	}
}

// Refusing to *sign* with an expired key must not stop us *verifying* tokens
// it already signed — those stay valid until their own exp.
func TestLookupStillResolvesAnExpiredKey(t *testing.T) {
	now := time.Now()
	r := ring(t, accessKey(t, "ak_1", now.Add(-90*24*time.Hour), now.Add(-time.Minute)))

	if _, err := r.Lookup("ak_1"); err != nil {
		t.Fatalf("verification lookup must still resolve a retired-by-time key: %v", err)
	}
}

func TestActiveAccessReportsNoKeyDistinctlyFromOutOfWindow(t *testing.T) {
	empty := ring(t)
	if _, err := empty.ActiveAccessAt(time.Now()); !errors.Is(err, ErrNoActiveKey) {
		t.Fatalf("err = %v, want ErrNoActiveKey", err)
	}
}
