package apiconnect

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
	"github.com/gsoultan/anubis/pkg/anubis"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

type linkTenants struct{ tenancyport.TenantRepository }

func (linkTenants) TenantBySlug(_ context.Context, slug string) (*tenancydomain.TenantRef, error) {
	if slug != "acme" {
		return nil, errors.New("no")
	}
	return &tenancydomain.TenantRef{ID: "t1", Slug: "acme"}, nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// An access-link token was attached as a full principal on every procedure,
// and only the decision endpoint and the gate looked at its amr: whoever held
// the link could share any record the recipient could reach, list and end the
// recipient's sessions, or bind an authenticator to their account. A link
// token now authenticates the decision endpoint and nothing else.
func TestALinkTokenAuthenticatesOnlyTheDecisionEndpoint(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ring, err := keyring.NewRing([]*keyring.Key{{
		Kid: "k1", Purpose: keyring.PurposeAccess, Status: keyring.StatusActive, Public: pub, Private: priv,
	}})
	if err != nil {
		t.Fatal(err)
	}
	sign := func(amr []string) string {
		body, _ := json.Marshal(anubis.Claims{
			Issuer: "https://id.example.test", Subject: "rudi", Audience: []string{"anubis"},
			Expires: now.Add(10 * time.Minute).Unix(), IssuedAt: now.Unix(), NotBefore: now.Unix(),
			Tenant: "acme", AMR: amr, AuthTime: now.Unix(),
			Scopes: map[string]string{"_record": "job_requisition/JR-0142"},
		})
		footer, _ := json.Marshal(map[string]string{"kid": "k1"})
		tok, err := paseto.Sign(priv, body, footer, nil)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	i := NewAuthnInterceptor("https://id.example.test", keyring.NewManager(ring), linkTenants{}, nil, nil, fixedClock{now})

	attached := func(procedure, token string) bool {
		h := http.Header{}
		h.Set("Authorization", "Bearer "+token)
		_, ok := authctx.From(i.authenticate(context.Background(), procedure, h))
		return ok
	}

	link := sign([]string{"link"})
	if !attached(anubisv1connect.AuthzServiceAuthorizeProcedure, link) {
		t.Fatal("a link token must still reach the decision endpoint")
	}
	for _, proc := range []string{
		anubisv1connect.ShareServiceShareProcedure,
		anubisv1connect.ShareServiceCreateAccessLinkProcedure,
		anubisv1connect.SessionServiceListSessionsProcedure,
		anubisv1connect.AuthzServiceExplainProcedure,
		anubisv1connect.AuthzServiceSwitchScopeProcedure,
	} {
		if attached(proc, link) {
			t.Errorf("%s: a link token was accepted as a sign-in", proc)
		}
	}
	// A token from a real sign-in is untouched.
	if !attached(anubisv1connect.ShareServiceShareProcedure, sign([]string{"pwd"})) {
		t.Fatal("a signed-in token stopped authenticating")
	}
}
