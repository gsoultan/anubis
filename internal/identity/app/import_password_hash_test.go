package identityapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// A bcrypt hash of "allmine" — the vector golang.org/x/crypto pins itself to.
const importedBcrypt = "$2a$10$XajjQvNhvvRt5GSeFk1xFeyqRrsxkhBkUiQeg0dt.wU1qD4aFDcga"

// Stubs embed their port so a method this test does not exercise panics
// naming itself rather than returning a zero value that looks like a result.
type importOps struct{}

func (importOps) AssignmentsForOperator(context.Context, string) ([]controldomain.AssignmentRecord, error) {
	return []controldomain.AssignmentRecord{{OperatorID: "op", TenantID: "tenant-1", Role: controldomain.RoleAdmin}}, nil
}

func (importOps) OperatorStanding(context.Context, string) (bool, int, error) { return true, 0, nil }

type importIdentities struct {
	identityport.IdentityRepository
}

func (importIdentities) Identity(_ context.Context, tenantID, id string) (*identitydomain.Identity, error) {
	if id != "identity-1" || tenantID != "tenant-1" {
		return nil, errors.New("no such identity")
	}
	return &identitydomain.Identity{ID: id, TenantID: tenantID, Status: "active"}, nil
}

type importCreds struct {
	identityport.CredentialRepository
	revokedKinds []string
	created      []credential.CredentialInput
}

func (c *importCreds) RevokeCredentialsOfKind(_ context.Context, _, kind string) (int64, error) {
	c.revokedKinds = append(c.revokedKinds, kind)
	return 1, nil
}

func (c *importCreds) CreateCredential(_ context.Context, in credential.CredentialInput) (string, error) {
	c.created = append(c.created, in)
	return "cred-new", nil
}

type importTx struct{}

func (importTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type importClock struct{}

func (importClock) Now() time.Time { return time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC) }

type importAudit struct{ events []auditdomain.AuditEvent }

func (a *importAudit) Emit(_ context.Context, ev auditdomain.AuditEvent) {
	a.events = append(a.events, ev)
}

func newImporter(t *testing.T) (CredentialAdminUsecase, *importCreds, *importAudit) {
	t.Helper()
	creds := &importCreds{}
	audit := &importAudit{}
	u := NewIdentityAdminInteractor(importOps{}, importClock{}.Now, nil, importIdentities{}, creds,
		nil, nil, nil, nil, nil, nil, importTx{}, importClock{}, audit)
	return u, creds, audit
}

func operatorCtx() context.Context {
	return authctx.With(context.Background(), &authctx.Principal{
		IdentityID: "op", TenantID: "tenant-1", Platform: true,
	})
}

// The cut-over call: a hash from the old system becomes the person's
// password here, replacing whatever they held, with the expiry the operator
// gave it — and the audit trail says what arrived.
func TestAnImportedHashReplacesThePasswordWithItsExpiry(t *testing.T) {
	u, creds, audit := newImporter(t)
	expires := importClock{}.Now().Add(30 * 24 * time.Hour)

	if err := u.ImportPasswordHash(operatorCtx(), "identity-1", importedBcrypt, expires); err != nil {
		t.Fatalf("import refused: %v", err)
	}

	if len(creds.revokedKinds) != 1 || creds.revokedKinds[0] != "password" {
		t.Fatalf("the previous password was not retired: %v", creds.revokedKinds)
	}
	if len(creds.created) != 1 {
		t.Fatalf("expected one credential, got %d", len(creds.created))
	}
	c := creds.created[0]
	if c.Kind != "password" || c.Secret != importedBcrypt || c.IdentityID != "identity-1" || c.TenantID != "tenant-1" {
		t.Fatalf("stored credential is wrong: %+v", c)
	}
	if c.ExpiresAt == nil || !c.ExpiresAt.Equal(expires) {
		t.Fatalf("expiry not carried: %v", c.ExpiresAt)
	}
	var seen bool
	for _, ev := range audit.events {
		if ev.Action == "credential.password_imported" && ev.TargetID == "identity-1" &&
			strings.Contains(string(ev.Detail), `"algorithm":"bcrypt"`) {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("import not audited with its algorithm: %+v", audit.events)
	}
}

// The mistake this call will actually see is a PASSWORD pasted where the hash
// goes. Stored, it would be a secret nobody can verify and anybody with the
// table can read. Refused before anything is written.
func TestAPlaintextOrUnknownStringIsRefusedBeforeAnyWrite(t *testing.T) {
	u, creds, _ := newImporter(t)
	expires := importClock{}.Now().Add(time.Hour)

	for _, s := range []string{"hunter2-is-not-a-hash", "", "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$ZGs",
		"$2a$10$tooshort"} {
		err := u.ImportPasswordHash(operatorCtx(), "identity-1", s, expires)
		if apperr.AsError(err).Code != "invalid_argument" {
			t.Errorf("%q: want invalid_argument, got %v", s, err)
		}
	}
	if len(creds.created)+len(creds.revokedKinds) != 0 {
		t.Fatal("a refused import wrote to the credential table")
	}
}

// A legacy hash is a bridge: it carries the source's cost, so it must carry
// an end date, and not a distant one.
func TestALegacyHashMustExpireWithinNinetyDays(t *testing.T) {
	u, creds, _ := newImporter(t)
	now := importClock{}.Now()

	cases := map[string]time.Time{
		"no expiry":   {},
		"in the past": now.Add(-time.Minute),
		"91 days out": now.Add(91 * 24 * time.Hour),
		"a year out":  now.Add(365 * 24 * time.Hour),
	}
	for name, exp := range cases {
		err := u.ImportPasswordHash(operatorCtx(), "identity-1", importedBcrypt, exp)
		if apperr.AsError(err).Code != "invalid_argument" {
			t.Errorf("%s: want invalid_argument, got %v", name, err)
		}
	}
	if len(creds.created) != 0 {
		t.Fatal("a refused import stored a credential")
	}
	// The boundary itself is allowed.
	if err := u.ImportPasswordHash(operatorCtx(), "identity-1", importedBcrypt, now.Add(90*24*time.Hour)); err != nil {
		t.Fatalf("90 days refused: %v", err)
	}
}

// A hash in Anubis's own format is an export from another installation, not
// a weak bridge: it may live without an end date.
func TestANativeHashMayBeImportedWithoutExpiry(t *testing.T) {
	u, creds, _ := newImporter(t)
	native, err := kdf.Hash("moving-between-installations")
	if err != nil {
		t.Fatal(err)
	}
	if err := u.ImportPasswordHash(operatorCtx(), "identity-1", native, time.Time{}); err != nil {
		t.Fatalf("native import refused: %v", err)
	}
	if len(creds.created) != 1 || creds.created[0].ExpiresAt != nil {
		t.Fatalf("native import stored wrongly: %+v", creds.created)
	}
}

// Nobody without the platform credential-write permission imports anything —
// the same line SetPassword holds.
func TestAnImportNeedsAnOperator(t *testing.T) {
	u, creds, _ := newImporter(t)
	tenantUser := authctx.With(context.Background(), &authctx.Principal{IdentityID: "someone", TenantID: "tenant-1"})
	err := u.ImportPasswordHash(tenantUser, "identity-1", importedBcrypt, importClock{}.Now().Add(time.Hour))
	if code := apperr.AsError(err).Code; code != "permission_denied" {
		t.Fatalf("want permission_denied, got %v", err)
	}
	err = u.ImportPasswordHash(context.Background(), "identity-1", importedBcrypt, importClock{}.Now().Add(time.Hour))
	if code := apperr.AsError(err).Code; code != "unauthenticated" {
		t.Fatalf("want unauthenticated, got %v", err)
	}
	if len(creds.created) != 0 {
		t.Fatal("a refused caller stored a credential")
	}
}
