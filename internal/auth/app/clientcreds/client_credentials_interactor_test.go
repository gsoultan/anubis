package clientcreds

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

const clientSecret = "a-client-secret-only-the-service-holds"

type ccTenants struct {
	tenancyport.TenantRepository
	status string
}

func (s ccTenants) TenantBySlug(_ context.Context, slug string) (*tenancydomain.TenantRef, error) {
	return &tenancydomain.TenantRef{ID: "t1", Slug: slug, Status: s.status}, nil
}

type ccApps struct{ tenancyport.ApplicationRepository }

func (ccApps) ApplicationBySlug(_ context.Context, _, slug string) (*tenancydomain.ApplicationRecord, error) {
	return &tenancydomain.ApplicationRecord{ID: "app-1", Slug: slug, Status: "active",
		ClientSecretHash: hex.EncodeToString(secret.Hash(clientSecret))}, nil
}

type ccClock struct{}

func (ccClock) Now() time.Time { return time.Unix(1_700_000_000, 0) }

type ccAudit struct{}

func (ccAudit) Emit(context.Context, auditdomain.AuditEvent) {}

// A suspended tenant's systems get no token, as its people and its API keys
// get none.
//
// Root cause: the tenant was resolved by slug for its id alone, so its status
// never reached the decision and a suspended tenant's services kept minting.
func TestASuspendedTenantsServiceGetsNoToken(t *testing.T) {
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
	mint := func(status string) error {
		uc := NewClientCredentialsInteractor("https://id.example.test", keyring.NewManager(ring),
			ccApps{}, ccTenants{status: status}, ccClock{}, ccAudit{})
		_, err := uc.Execute(context.Background(), ClientCredentialsInput{
			Tenant: "acme", ClientID: "billing", ClientSecret: clientSecret,
		})
		return err
	}
	if err := mint("active"); err != nil {
		t.Fatalf("an active tenant's service was refused: %v", err)
	}
	for _, status := range []string{"suspended", "archived"} {
		if mint(status) == nil {
			t.Errorf("a %s tenant's service was issued a token", status)
		}
	}
}
