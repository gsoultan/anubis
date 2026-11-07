package apiconnect

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
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

type statusTenants struct {
	tenancyport.TenantRepository
	status string
}

func (s statusTenants) TenantBySlug(_ context.Context, slug string) (*tenancydomain.TenantRef, error) {
	return &tenancydomain.TenantRef{ID: "t1", Slug: slug, Status: s.status}, nil
}

// A token already minted for a tenant stops authenticating once the tenant is
// suspended or retired.
//
// Root cause: the interceptor resolved the token's tenant by slug and used
// only its id, so the tenant's status never reached the decision and a
// suspended tenant's tokens kept working until they expired. API keys already
// honoured it; tokens did not.
func TestATenantTokenStopsWhenTheTenantIsNotActive(t *testing.T) {
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
	body, _ := json.Marshal(anubis.Claims{
		Issuer: "https://id.example.test", Subject: "rudi", Audience: []string{"anubis"},
		Expires: now.Add(10 * time.Minute).Unix(), IssuedAt: now.Unix(), NotBefore: now.Unix(),
		Tenant: "acme", AMR: []string{"pwd"}, AuthTime: now.Unix(),
	})
	footer, _ := json.Marshal(map[string]string{"kid": "k1"})
	tok, err := paseto.Sign(priv, body, footer, nil)
	if err != nil {
		t.Fatal(err)
	}
	attached := func(status string) bool {
		i := NewAuthnInterceptor("https://id.example.test", keyring.NewManager(ring),
			statusTenants{status: status}, nil, nil, fixedClock{now})
		h := http.Header{}
		h.Set("Authorization", "Bearer "+tok)
		_, ok := authctx.From(i.authenticate(context.Background(),
			anubisv1connect.AuthzServiceAuthorizeProcedure, h))
		return ok
	}
	if !attached("active") {
		t.Fatal("an active tenant's token stopped authenticating")
	}
	for _, status := range []string{"suspended", "archived"} {
		if attached(status) {
			t.Errorf("a %s tenant's token still authenticates", status)
		}
	}
}
