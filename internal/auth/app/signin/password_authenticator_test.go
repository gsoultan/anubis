package signin

import (
	"context"
	"errors"
	"testing"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// Stubs embed their port so a method this test does not exercise is a nil
// panic naming itself, rather than a silent zero value that changes what is
// being measured.
type stubTenants struct {
	tenancyport.TenantRepository
	ref *tenancydomain.TenantRef
}

func (s stubTenants) TenantBySlug(context.Context, string) (*tenancydomain.TenantRef, error) {
	return s.ref, nil
}

type stubRealms struct {
	identityport.RealmRepository
	realm *identitydomain.Realm
}

func (s stubRealms) RealmByCode(context.Context, string, string) (*identitydomain.Realm, error) {
	return s.realm, nil
}

type stubIdentities struct {
	identityport.IdentityRepository
	identity *identitydomain.Identity
}

func (s stubIdentities) IdentityForLogin(context.Context, string, string, string) (*identitydomain.Identity, error) {
	return s.identity, nil
}

type stubCreds struct {
	identityport.CredentialRepository
	password *credential.Credential
	kinds    []string
	kindsErr error
}

func (s stubCreds) PasswordCredential(context.Context, string) (*credential.Credential, error) {
	return s.password, nil
}

func (s stubCreds) ActiveCredentialKinds(context.Context, string) ([]string, error) {
	return s.kinds, s.kindsErr
}

type stubAuditor struct{ events []auditdomain.AuditEvent }

func (s *stubAuditor) Emit(_ context.Context, ev auditdomain.AuditEvent) {
	s.events = append(s.events, ev)
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }

const probePassword = "a-password-worth-checking-1234"

func newProbe(t *testing.T, creds stubCreds) (*PasswordAuthenticator, *stubAuditor) {
	t.Helper()
	hash, err := kdf.Hash(probePassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	creds.password = &credential.Credential{ID: "cred-1", Kind: "password", Secret: hash}
	audit := &stubAuditor{}
	return NewPasswordAuthenticator(
		stubTenants{ref: &tenancydomain.TenantRef{ID: "tenant-1", Slug: "probe"}},
		stubRealms{realm: &identitydomain.Realm{
			ID: "realm-1", Code: "internal", AllowedFactors: []string{"password", "totp"},
		}},
		stubIdentities{identity: &identitydomain.Identity{ID: "identity-1", Status: "active"}},
		creds, fixedClock{}, audit,
	), audit
}

// A second factor we could not look up must not be treated as one that is
// not there.
//
// This is the one place the two login doors disagreed and the browser was
// right: the hosted page answered "a factor exists" when the credential read
// failed, and the interactor returned nil and carried on to issue a session.
// Merging them into one implementation could have spread the weaker answer to
// both, which is why the merge took the stricter one and why this test holds
// it there.
//
// The failure it prevents: a transient database error during the credential
// read signs somebody in with a password alone, on an identity that has an
// authenticator enrolled. It looks exactly like a successful login.
func TestAFactorLookupFailureRefusesRatherThanAdmits(t *testing.T) {
	auth, audit := newProbe(t, stubCreds{kindsErr: errors.New("connection reset")})

	d := auth.Authenticate(context.Background(), LoginInput{
		Tenant: "probe", Realm: "internal", Username: "someone", Password: probePassword,
	}, SurfaceAPI)

	if d.Step != StepDeny {
		t.Fatalf("a failed factor lookup produced step %v, want StepDeny — "+
			"the password alone was enough", d.Step)
	}
	if d.Err == nil {
		t.Fatal("refused without an error to return")
	}
	// It must be distinguishable from a wrong password: telling somebody
	// their password is wrong when the server could not check sends them to
	// reset a password that was fine.
	if d.Err.Error() == "" {
		t.Fatal("refusal carries no reason")
	}
	var refused bool
	for _, ev := range audit.events {
		if ev.Action == "auth.login" && ev.Result == "deny" {
			refused = true
		}
	}
	if !refused {
		t.Fatal("the refusal was not audited; an operator cannot see it happened")
	}
}

// And the correct password still gets in when the lookup works, or the guard
// above would be indistinguishable from refusing everybody.
func TestACleanFactorLookupStillAdmits(t *testing.T) {
	auth, _ := newProbe(t, stubCreds{kinds: nil})

	d := auth.Authenticate(context.Background(), LoginInput{
		Tenant: "probe", Realm: "internal", Username: "someone", Password: probePassword,
	}, SurfaceAPI)

	if d.Step != StepAllow {
		t.Fatalf("a correct password with nothing enrolled produced step %v, want StepAllow", d.Step)
	}
}

// An enrolled factor the realm still accepts is demanded, whichever door
// asked — the property both doors now share by construction.
func TestAnEnrolledFactorIsDemanded(t *testing.T) {
	auth, _ := newProbe(t, stubCreds{kinds: []string{"totp"}})

	for _, surface := range []string{SurfaceAPI, SurfaceBrowser} {
		d := auth.Authenticate(context.Background(), LoginInput{
			Tenant: "probe", Realm: "internal", Username: "someone", Password: probePassword,
		}, surface)
		if d.Step != StepFactor {
			t.Fatalf("%s: step %v, want StepFactor", surface, d.Step)
		}
		if len(d.Methods) != 1 || d.Methods[0] != "totp" {
			t.Fatalf("%s: methods %v, want [totp]", surface, d.Methods)
		}
	}
}
