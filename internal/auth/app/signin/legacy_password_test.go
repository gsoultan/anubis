package signin

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

// A bcrypt hash of "allmine" — the vector golang.org/x/crypto pins itself to.
// Imported as it is from the system being replaced; the test must not MAKE
// one, because nothing in this tree is allowed to.
const (
	legacyPassword = "allmine"
	legacyHash     = "$2a$10$XajjQvNhvvRt5GSeFk1xFeyqRrsxkhBkUiQeg0dt.wU1qD4aFDcga"
)

// recordingCreds is stubCreds that remembers every write, so a test can say
// which shape the upgrade took rather than only that sign-in succeeded.
type recordingCreds struct {
	stubCreds
	created []credential.CredentialInput
	revoked []string
	updated []string
}

func (r *recordingCreds) PasswordCredential(context.Context, string) (*credential.Credential, error) {
	return r.password, nil
}

func (r *recordingCreds) ActiveCredentialKinds(context.Context, string) ([]string, error) {
	return r.kinds, r.kindsErr
}

func (r *recordingCreds) CreateCredential(_ context.Context, in credential.CredentialInput) (string, error) {
	r.created = append(r.created, in)
	return "cred-new", nil
}

func (r *recordingCreds) RevokeCredential(_ context.Context, _, id string) error {
	r.revoked = append(r.revoked, id)
	return nil
}

func (r *recordingCreds) UpdateCredentialSecret(_ context.Context, id, _, _ string) error {
	r.updated = append(r.updated, id)
	return nil
}

func newLegacyProbe(t *testing.T, cred *credential.Credential) (*PasswordAuthenticator, *recordingCreds, *stubAuditor) {
	t.Helper()
	creds := &recordingCreds{}
	creds.password = cred
	audit := &stubAuditor{}
	return NewPasswordAuthenticator(
		stubTenants{ref: &tenancydomain.TenantRef{ID: "tenant-1", Slug: "probe"}},
		stubRealms{realm: &identitydomain.Realm{
			ID: "realm-1", Code: "internal", AllowedFactors: []string{"password"},
		}},
		stubIdentities{identity: &identitydomain.Identity{ID: "identity-1", Status: "active"}},
		creds, fixedClock{}, audit,
	), creds, audit
}

func login(auth *PasswordAuthenticator, password string) Decision {
	return auth.Authenticate(context.Background(), LoginInput{
		Tenant: "probe", Realm: "internal", Username: "someone", Password: password,
	}, SurfaceAPI)
}

// The whole point of importing a hash: the person signs in with the password
// they already know, and leaves with one at this installation's cost.
//
// The upgrade is a REPLACEMENT, not an edit. The imported row carries the
// expiry its bcrypt hash was given; writing PBKDF2 into that row would hand
// the new hash the old deadline, and the person would be locked out on the
// day the migration was meant to be over.
func TestAnImportedHashSignsInOnceAndIsReplacedWithoutItsExpiry(t *testing.T) {
	expires := fixedClock{}.Now().Add(30 * 24 * time.Hour)
	auth, creds, _ := newLegacyProbe(t, &credential.Credential{
		ID: "cred-legacy", IdentityID: "identity-1", Kind: "password",
		Secret: legacyHash, ExpiresAt: &expires,
	})

	if d := login(auth, legacyPassword); d.Step != StepAllow {
		t.Fatalf("the imported password was refused: step %v err %v", d.Step, d.Err)
	}

	if len(creds.updated) != 0 {
		t.Fatalf("the imported row was edited in place (%v); its expiry now applies to the new hash", creds.updated)
	}
	if len(creds.created) != 1 {
		t.Fatalf("expected one replacement credential, got %d", len(creds.created))
	}
	fresh := creds.created[0]
	if fresh.Kind != "password" || fresh.IdentityID != "identity-1" || fresh.TenantID != "tenant-1" {
		t.Fatalf("replacement credential is mis-attributed: %+v", fresh)
	}
	if !strings.HasPrefix(fresh.Secret, "$pbkdf2-sha256$") {
		t.Fatalf("replacement is not at this installation's cost: %q", fresh.Secret[:16])
	}
	if ok, rehash, err := kdf.Verify(legacyPassword, fresh.Secret); err != nil || !ok || rehash {
		t.Fatalf("replacement does not verify the same password cleanly: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	if fresh.ExpiresAt != nil {
		t.Fatalf("replacement inherited the legacy expiry %v", fresh.ExpiresAt)
	}
	if len(creds.revoked) != 1 || creds.revoked[0] != "cred-legacy" {
		t.Fatalf("the imported row was not retired: revoked %v", creds.revoked)
	}
}

// A native hash that merely needs a cost upgrade keeps the old shape: the
// same row, edited. Pinned so the replacement path cannot quietly become the
// only path and start churning credential ids on every policy bump.
func TestANativeHashUpgradesInPlace(t *testing.T) {
	weak, err := hashAtFewerIterations(probePassword)
	if err != nil {
		t.Fatal(err)
	}
	auth, creds, _ := newLegacyProbe(t, &credential.Credential{
		ID: "cred-weak", IdentityID: "identity-1", Kind: "password", Secret: weak,
	})

	if d := login(auth, probePassword); d.Step != StepAllow {
		t.Fatalf("refused: step %v err %v", d.Step, d.Err)
	}
	if len(creds.updated) != 1 || creds.updated[0] != "cred-weak" {
		t.Fatalf("native upgrade did not edit the row in place: updated %v", creds.updated)
	}
	if len(creds.created) != 0 || len(creds.revoked) != 0 {
		t.Fatalf("native upgrade replaced the row: created %d revoked %v", len(creds.created), creds.revoked)
	}
}

// Past its expiry an imported hash is dead, even with the right password.
// The person is told their password is wrong — "your imported password has
// lapsed" also says "this account exists and came from the old system" —
// and the audit record carries the real reason for the operator.
func TestAnExpiredImportedHashIsRefusedWithTheRightPassword(t *testing.T) {
	expired := fixedClock{}.Now().Add(-time.Minute)
	auth, creds, audit := newLegacyProbe(t, &credential.Credential{
		ID: "cred-legacy", IdentityID: "identity-1", Kind: "password",
		Secret: legacyHash, ExpiresAt: &expired,
	})

	d := login(auth, legacyPassword)
	if d.Step != StepDeny {
		t.Fatalf("an expired imported hash signed in: step %v", d.Step)
	}
	if !errors.Is(d.Err, apperr.ErrInvalidCredentials) {
		t.Fatalf("refusal names the real reason to the caller: %v", d.Err)
	}
	if len(creds.created)+len(creds.updated)+len(creds.revoked) != 0 {
		t.Fatal("a refused sign-in wrote to the credential table")
	}
	var audited bool
	for _, ev := range audit.events {
		if ev.Action == "auth.login" && ev.Result == "deny" && strings.Contains(string(ev.Detail), "credential_expired") {
			audited = true
		}
	}
	if !audited {
		t.Fatalf("the operator cannot tell this lapse from a typo: %+v", audit.events)
	}
}

// And a wrong password against an imported hash is still just a wrong
// password — no replacement, no revocation.
func TestAWrongPasswordAgainstAnImportedHashChangesNothing(t *testing.T) {
	expires := fixedClock{}.Now().Add(30 * 24 * time.Hour)
	auth, creds, _ := newLegacyProbe(t, &credential.Credential{
		ID: "cred-legacy", IdentityID: "identity-1", Kind: "password",
		Secret: legacyHash, ExpiresAt: &expires,
	})

	if d := login(auth, "not-"+legacyPassword); d.Step != StepDeny {
		t.Fatalf("wrong password admitted: step %v", d.Step)
	}
	if len(creds.created)+len(creds.updated)+len(creds.revoked) != 0 {
		t.Fatal("a wrong password wrote to the credential table")
	}
}

// hashAtFewerIterations makes a native hash the installation would upgrade.
// It is assembled from the DOCUMENTED format ($pbkdf2-sha256$i=N$salt$dk,
// kdf package comment) rather than a package hook, because a hook that
// hashes at a chosen cost is exactly the function production must not have.
func hashAtFewerIterations(password string) (string, error) {
	salt := []byte("sixteen-byte-salt")[:16]
	dk, err := pbkdf2.Key(sha256.New, password, salt, 1000, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return "$pbkdf2-sha256$i=1000$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(dk), nil
}
