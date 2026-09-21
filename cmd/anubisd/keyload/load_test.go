package keyload

import (
	"context"
	"crypto/ed25519"
	"io"
	"log/slog"
	"testing"
	"time"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
)

// stubKeys is the read buildRing makes and nothing else.
type stubKeys struct {
	records []authdomain.KeyRecord
}

func (s stubKeys) VerificationKeys(context.Context) ([]authdomain.KeyRecord, error) {
	return s.records, nil
}
func (s stubKeys) CreateKey(context.Context, authdomain.KeyRecord) error       { panic("unused") }
func (s stubKeys) PromotePending(context.Context, string) (int64, error)       { panic("unused") }
func (s stubKeys) DemoteActive(context.Context, string) (int64, error)         { panic("unused") }
func (s stubKeys) SetKeyStatus(context.Context, string, string) error          { panic("unused") }
func (s stubKeys) SigningKeys(context.Context) ([]authdomain.KeyRecord, error) { panic("unused") }

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// sealedUnder returns a key record whose private half is sealed under master.
func sealedUnder(t *testing.T, master []byte, kid, status string) authdomain.KeyRecord {
	t.Helper()
	k, err := keyring.GenerateAccessKey(time.Now(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := keyring.SealSecret(master, kid, k.Private.Seed())
	if err != nil {
		t.Fatal(err)
	}
	return authdomain.KeyRecord{
		Kid: kid, Alg: k.Alg, Status: status, Purpose: keyring.PurposeAccess,
		PublicKey: k.Public, PrivateKeyEnc: sealed,
		NotBefore: k.NotBefore, NotAfter: k.NotAfter,
	}
}

// A retiring key that cannot be unsealed must not stop the server booting.
//
// It is loaded to keep VERIFYING tokens signed before the rotation, and
// verification uses the public half, which is stored in plaintext beside it.
// NewRing only ever selects an active key to sign with, so a retiring one
// without its private half cannot be picked by accident.
//
// Treating it as fatal turned a botched re-seal into a total outage. Observed:
// three keys left behind by `anubisd keys reseal` under a different master key
// stopped the server starting at all, and the only thing genuinely lost was
// the ability to sign with keys that must not sign.
func TestARetiringKeyThatWillNotUnsealDoesNotStopTheBoot(t *testing.T) {
	ours := make([]byte, 32)
	theirs := make([]byte, 32)
	theirs[0] = 1 // a different master: the reseal case, exactly

	good := sealedUnder(t, ours, "ak_active", keyring.StatusActive)
	stale := sealedUnder(t, theirs, "zzreseal-1789823511719668000", keyring.StatusRetiring)

	ring, err := buildRing(context.Background(), quiet(),
		stubKeys{records: []authdomain.KeyRecord{good, stale}}, ours)
	if err != nil {
		t.Fatalf("one unopenable RETIRING key stopped the boot: %v", err)
	}

	// The active key still signs.
	active, err := ring.ActiveAccessPresent()
	if err != nil {
		t.Fatalf("active key missing after loading a stale retiring one: %v", err)
	}
	if len(active.Private) != ed25519.PrivateKeySize {
		t.Fatal("the active key was loaded without its private half")
	}
	// The retiring key still verifies, and carries nothing to sign with.
	old, err := ring.Lookup("zzreseal-1789823511719668000")
	if err != nil {
		t.Fatalf("the retiring key was dropped, so tokens it signed stop verifying: %v", err)
	}
	if len(old.Public) != ed25519.PublicKeySize {
		t.Fatal("the retiring key kept no public half, which is the only reason to load it")
	}
	if old.Private != nil {
		t.Fatal("a key that would not unseal came back with private material")
	}
}

// The other half: a key that must still SIGN is fatal, because booting
// without it serves an error on every login instead of saying why once.
func TestAnActiveKeyThatWillNotUnsealStopsTheBoot(t *testing.T) {
	ours := make([]byte, 32)
	theirs := make([]byte, 32)
	theirs[0] = 1

	for _, status := range []string{keyring.StatusActive, keyring.StatusPending} {
		bad := sealedUnder(t, theirs, "ak_"+status, status)
		if _, err := buildRing(context.Background(), quiet(),
			stubKeys{records: []authdomain.KeyRecord{bad}}, ours); err == nil {
			t.Fatalf("a %s key that could not be unsealed booted anyway", status)
		}
	}
}
