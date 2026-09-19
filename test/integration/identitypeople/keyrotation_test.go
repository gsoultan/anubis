//go:build integration

package identitypeople

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	credentialdomain "github.com/gsoultan/anubis/internal/identity/domain/credential"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
)

// ringWith builds a ring whose ACTIVE local key is the last one given; every
// earlier key is retiring, which is what `anubisd keys promote local` leaves
// behind (DemoteActive then PromotePending).
func ringWith(t *testing.T, keys ...*keyring.Key) *keyring.Ring {
	t.Helper()
	set := make([]*keyring.Key, len(keys))
	for i, k := range keys {
		c := *k
		c.Status = keyring.StatusRetiring
		if i == len(keys)-1 {
			c.Status = keyring.StatusActive
		}
		set[i] = &c
	}
	r, err := keyring.NewRing(set)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func localKey(t *testing.T) *keyring.Key {
	t.Helper()
	k, err := keyring.GenerateLocalKey(time.Now(), 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// A TOTP secret is sealed ONCE at enrolment and read for the life of the
// enrolment. Rotating the local signing key must not make it unreadable.
//
// docs/operations.md documents `keys prepare local && keys promote local` as
// routine, and requires it after a key compromise. If rotation silently
// breaks the second factor, every enrolled user is locked out at exactly the
// moment the system is under attack — and the failure presents as "invalid
// code", which reads as user error rather than as an outage.
func TestTOTPSecretSurvivesLocalKeyRotation(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	id := newIdentity(t, r, fmt.Sprintf("zzrot%d", time.Now().UnixNano()%1_000_000))
	credID, err := r.CreateCredential(ctx, credentialdomain.CredentialInput{
		IdentityID: id, TenantID: tenant, Kind: "totp", Params: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	shared := []byte("0123456789abcdef0123456789abcdef")

	// Enrolment, as enrollment_interactor.go does it: seal under whichever
	// local key is active now.
	keyA := localKey(t)
	ringA := ringWith(t, keyA)
	active, err := ringA.ActiveLocal()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := keyring.SealSecret(active.Secret, "totp:"+credID, shared)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateCredentialSecret(ctx, credID,
		base64.RawStdEncoding.EncodeToString(sealed), active.Kid); err != nil {
		t.Fatal(err)
	}

	// Rotation: a second local key is prepared and promoted. The old one is
	// retiring, so it is still IN the ring — this is the gentlest possible
	// version of the scenario.
	keyB := localKey(t)
	ringB := ringWith(t, keyA, keyB)

	// Verification, after the rotation.
	cred, err := r.ActiveCredentialOfKind(ctx, id, "totp")
	if err != nil {
		t.Fatal(err)
	}
	if cred == nil {
		t.Fatal("the credential vanished")
	}
	raw, err := base64.RawStdEncoding.DecodeString(cred.Secret)
	if err != nil {
		t.Fatal(err)
	}

	got, err := openTOTP(ringB, cred, raw)
	if err != nil {
		t.Fatalf("a second factor enrolled before a key rotation can no longer be read: %v", err)
	}
	if string(got) != string(shared) {
		t.Fatalf("the secret came back changed")
	}
}

// openTOTP is what the verify path does to recover the shared secret: open
// under the key the secret NAMES, not whichever is active now.
func openTOTP(ring *keyring.Ring, cred *credentialdomain.Credential, sealed []byte) ([]byte, error) {
	m, _, err := keyring.OpenNamedSecret(ring, cred.SecretKid, "totp:"+cred.ID, sealed)
	return m, err
}

// Enrolments written before secret_kid existed must keep working, and must
// stop being a guess: the first successful read records which key opened
// them, so the NEXT rotation has an answer.
func TestPreExistingEnrolmentHealsOnFirstRead(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	id := newIdentity(t, r, fmt.Sprintf("zzheal%d", time.Now().UnixNano()%1_000_000))
	credID, err := r.CreateCredential(ctx, credentialdomain.CredentialInput{
		IdentityID: id, TenantID: tenant, Kind: "totp", Params: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	shared := []byte("0123456789abcdef0123456789abcdef")
	keyA := localKey(t)
	ringA := ringWith(t, keyA)
	active, err := ringA.ActiveLocal()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := keyring.SealSecret(active.Secret, "totp:"+credID, shared)
	if err != nil {
		t.Fatal(err)
	}
	// The old shape: secret stored, kid NOT recorded.
	if err := r.UpdateCredentialSecret(ctx, credID,
		base64.RawStdEncoding.EncodeToString(sealed), ""); err != nil {
		t.Fatal(err)
	}

	cred, err := r.ActiveCredentialOfKind(ctx, id, "totp")
	if err != nil {
		t.Fatal(err)
	}
	if cred.SecretKid != "" {
		t.Fatalf("expected no kid on a pre-existing row, got %q", cred.SecretKid)
	}

	raw, _ := base64.RawStdEncoding.DecodeString(cred.Secret)
	got, usedKid, err := keyring.OpenNamedSecret(ringA, cred.SecretKid, "totp:"+cred.ID, raw)
	if err != nil {
		t.Fatalf("a pre-existing enrolment stopped working: %v", err)
	}
	if string(got) != string(shared) {
		t.Fatal("the secret came back changed")
	}

	// The heal: record what worked.
	if err := r.UpdateCredentialSecret(ctx, cred.ID, cred.Secret, usedKid); err != nil {
		t.Fatal(err)
	}
	healed, err := r.ActiveCredentialOfKind(ctx, id, "totp")
	if err != nil {
		t.Fatal(err)
	}
	if healed.SecretKid != keyA.Kid {
		t.Fatalf("kid recorded as %q, want %q", healed.SecretKid, keyA.Kid)
	}

	// And now it survives a rotation, which is the whole point of healing.
	ringB := ringWith(t, keyA, localKey(t))
	if _, _, err := keyring.OpenNamedSecret(ringB, healed.SecretKid, "totp:"+healed.ID, raw); err != nil {
		t.Fatalf("the healed row did not survive rotation: %v", err)
	}
}

// A key that has left the ring entirely is not a wrong code. The holder
// cannot fix it and no retry will help; the error has to name the key so the
// operator knows the enrolment must be redone.
func TestRetiredSealingKeySaysSo(t *testing.T) {
	keyA := localKey(t)
	sealed, err := keyring.SealSecret(keyA.Secret, "totp:probe", []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	// keyA is gone: retired keys are dropped from the ring entirely.
	ringB := ringWith(t, localKey(t))

	_, _, err = keyring.OpenNamedSecret(ringB, keyA.Kid, "totp:probe", sealed)
	if !errors.Is(err, keyring.ErrSealingKeyGone) {
		t.Fatalf("got %v, want ErrSealingKeyGone", err)
	}
	if !strings.Contains(err.Error(), keyA.Kid) {
		t.Fatalf("the error does not name the missing key: %v", err)
	}
}
