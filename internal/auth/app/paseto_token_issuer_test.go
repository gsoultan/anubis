package authapp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

type issuerClock struct{ t time.Time }

func (c issuerClock) Now() time.Time { return c.t }

type issuerTenants struct {
	tenancyport.TenantRepository
	status string
}

func (s issuerTenants) TenantByID(_ context.Context, id string) (*tenancydomain.TenantRef, error) {
	return &tenancydomain.TenantRef{ID: id, Slug: "acme", Status: s.status}, nil
}

type issuerRoles struct{ authzport.AuthzRepository }

func (issuerRoles) RolesForIdentity(context.Context, string, string) ([]string, error) {
	return nil, nil
}

// Suspending or retiring a tenant must stop its people getting tokens.
//
// Root cause: nothing on the way to a token read the tenant's status. Sign-in,
// refresh and scope switch all mint through Issue, which held the tenant
// repository and never asked it, so a suspended tenant's people kept signing
// in and refreshing as if nothing had happened (ADR-0011: suspending a tenant
// must invalidate its tokens).
func TestNoTokenIsIssuedForATenantThatIsNotActive(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ring, err := keyring.NewRing([]*keyring.Key{{
		Kid: "k1", Purpose: keyring.PurposeAccess, Status: keyring.StatusActive,
		Public: pub, Private: priv, NotBefore: now.Add(-time.Hour),
	}})
	if err != nil {
		t.Fatal(err)
	}
	issue := func(status string) (*TokenPair, error) {
		iss := NewPasetoTokenIssuer("https://id.example.test", keyring.NewManager(ring),
			nil, nil, nil, nil, issuerRoles{}, issuerTenants{status: status}, issuerClock{now})
		return iss.Issue(context.Background(), IssueInput{
			Session: &authdomain.SessionView{
				ID: "s1", IdentityID: "rudi", TenantID: "t1", IdentityStatus: "active",
				AMR: []string{"pwd"}, AuthTime: now,
			},
			TenantSlug: "acme",
			AccessOnly: true,
		})
	}

	if pair, err := issue("active"); err != nil || pair.AccessToken == "" {
		t.Fatalf("an active tenant got no token: %v", err)
	}
	for _, status := range []string{"suspended", "archived"} {
		pair, err := issue(status)
		if err == nil {
			t.Errorf("%s tenant was issued a token: %+v", status, pair)
			continue
		}
		if apperr.AsError(err).Code != "permission_denied" {
			t.Errorf("%s tenant: want permission denied, got %v", status, err)
		}
	}
}

// A person who is no longer active gets no token, however live their session.
//
// Root cause: the session read re-reads the identity's status on every call
// precisely so that it can outrank the session, and nothing looked at it.
// Disabling somebody also revokes their sessions, so that path was covered by
// accident; erasure and the retention sweep only disable and bump the epoch,
// and a refresh then minted a fresh token carrying the NEW epoch for a person
// who had been erased.
func TestNoTokenIsIssuedForAPersonWhoIsNotActive(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ring, err := keyring.NewRing([]*keyring.Key{{
		Kid: "k1", Purpose: keyring.PurposeAccess, Status: keyring.StatusActive,
		Public: pub, Private: priv, NotBefore: now.Add(-time.Hour),
	}})
	if err != nil {
		t.Fatal(err)
	}
	iss := NewPasetoTokenIssuer("https://id.example.test", keyring.NewManager(ring),
		nil, nil, nil, nil, issuerRoles{}, issuerTenants{status: "active"}, issuerClock{now})
	for _, status := range []string{"disabled", "locked", ""} {
		pair, err := iss.Issue(context.Background(), IssueInput{
			Session: &authdomain.SessionView{
				ID: "s1", IdentityID: "rudi", TenantID: "t1", IdentityStatus: status,
				AMR: []string{"pwd"}, AuthTime: now,
			},
			TenantSlug: "acme",
			AccessOnly: true,
		})
		if err == nil {
			t.Errorf("identity status %q was issued a token: %+v", status, pair)
		}
	}
}
