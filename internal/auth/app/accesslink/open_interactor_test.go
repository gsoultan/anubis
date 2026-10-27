package accesslink

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	"github.com/gsoultan/anubis/internal/authz/domain/share"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/accesstoken"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
	"github.com/gsoultan/anubis/pkg/anubis"
)

type stubTenants struct{ tenancyport.TenantRepository }

func (stubTenants) TenantBySlug(_ context.Context, slug string) (*tenancydomain.TenantRef, error) {
	if slug != "acme" {
		return nil, errors.New("no")
	}
	return &tenancydomain.TenantRef{ID: "t1", Slug: "acme"}, nil
}

type stubApps struct {
	tenancyport.ApplicationRepository
}

func (stubApps) ApplicationBySlug(_ context.Context, _, slug string) (*tenancydomain.ApplicationRecord, error) {
	if slug != "ats" {
		return nil, errors.New("no")
	}
	return &tenancydomain.ApplicationRecord{ID: "app-1", Slug: "ats", Status: "active"}, nil
}

type stubIdentities struct {
	identityport.IdentityRepository
}

func (stubIdentities) Identity(_ context.Context, _, id string) (*identitydomain.Identity, error) {
	switch id {
	case "rudi":
		return &identitydomain.Identity{ID: id, TenantID: "t1", Status: "active", TokenEpoch: 3, RealmCode: "public"}, nil
	case "gone":
		return &identitydomain.Identity{ID: id, TenantID: "t1", Status: "disabled", Disabled: true}, nil
	}
	return nil, errors.New("no")
}

type stubShares struct {
	authport.LinkedShareRepository
	byHash  map[string]*share.Share
	touched []string
}

func (s *stubShares) ShareByLinkHash(_ context.Context, hash []byte) (*share.Share, error) {
	if sh, ok := s.byHash[string(hash)]; ok {
		return sh, nil
	}
	return nil, apperr.ErrNotFound
}

func (s *stubShares) TouchShareLink(_ context.Context, id string) { s.touched = append(s.touched, id) }

type stubClock struct{}

func (stubClock) Now() time.Time { return time.Unix(1_700_000_000, 0) }

type stubAudit struct{ events []auditdomain.AuditEvent }

func (a *stubAudit) Emit(_ context.Context, ev auditdomain.AuditEvent) {
	a.events = append(a.events, ev)
}

func testRing(t *testing.T) (*keyring.Manager, ed25519.PublicKey) {
	t.Helper()
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
	return keyring.NewManager(ring), pub
}

func probe(t *testing.T, sh *share.Share) (OpenUsecase, string, *stubShares, *stubAudit, ed25519.PublicKey) {
	t.Helper()
	ring, pub := testRing(t)
	token := LinkPrefix + "a-link-only-the-recipient-was-sent"
	shares := &stubShares{byHash: map[string]*share.Share{}}
	if sh != nil {
		shares.byHash[string(secret.Hash(token))] = sh
	}
	audit := &stubAudit{}
	uc := NewOpenInteractor("https://id.example.test", ring, shares, stubTenants{}, stubApps{}, stubIdentities{}, stubClock{}, audit)
	return uc, token, shares, audit, pub
}

func liveShare(to string) *share.Share {
	return &share.Share{ID: "sh-1", TenantID: "t1", FromIdentityID: "dewi", ToIdentityID: to,
		Record: "job_requisition/JR-0142", Permissions: []string{"ats:requisition:comment"},
		ValidUntil: stubClock{}.Now().Add(7 * 24 * time.Hour)}
}

// What a link becomes, exactly: a short token for the recipient, for the
// named application, carrying amr "link" and the record — and no session.
func TestALinkBecomesARecordBoundTokenAndNothingMore(t *testing.T) {
	uc, token, shares, audit, pub := probe(t, liveShare("rudi"))

	pair, err := uc.OpenAccessLink(context.Background(), OpenInput{Tenant: "acme", ClientID: "ats", Token: token})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if pair.RefreshToken != "" || pair.SessionID != "" {
		t.Fatalf("a link minted a refresh token or a session: %+v", pair)
	}
	if pair.ExpiresIn > int(linkAccessTTL/time.Second) {
		t.Fatalf("token outlives the link budget: %d", pair.ExpiresIn)
	}
	msg, err := accesstoken.Verify(pub, pair.AccessToken)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	var c anubis.Claims
	if err := json.Unmarshal(msg, &c); err != nil {
		t.Fatal(err)
	}
	if c.Subject != "rudi" || c.Tenant != "acme" || len(c.Audience) != 1 || c.Audience[0] != "ats" ||
		c.Session != "" || c.Epoch != 3 || strings.Join(c.AMR, ",") != "link" ||
		c.Scopes["_record"] != "job_requisition/JR-0142" {
		t.Fatalf("claims wrong: %+v", c)
	}
	if len(shares.touched) != 1 || shares.touched[0] != "sh-1" {
		t.Fatalf("the open was not counted: %v", shares.touched)
	}
	var audited bool
	for _, ev := range audit.events {
		if ev.Action == "auth.link_open" && ev.Result == "allow" && ev.ActorID == "rudi" {
			audited = true
		}
	}
	if !audited {
		t.Fatal("the open was not audited")
	}
}

// A token must not outlive the share it came from.
func TestTheTokenEndsWithTheShare(t *testing.T) {
	sh := liveShare("rudi")
	sh.ValidUntil = stubClock{}.Now().Add(2 * time.Minute)
	uc, token, _, _, _ := probe(t, sh)
	pair, err := uc.OpenAccessLink(context.Background(), OpenInput{Tenant: "acme", ClientID: "ats", Token: token})
	if err != nil || pair.ExpiresIn > 120 {
		t.Fatalf("token expires in %d, share in 120: %v", pair.ExpiresIn, err)
	}
}

// Every refusal is "invalid link" to the caller and a named reason to the
// operator; a refusal mints nothing and counts nothing.
func TestEveryWrongLinkIsTheSameRefusal(t *testing.T) {
	cases := map[string]struct {
		share *share.Share
		in    OpenInput
	}{
		"no such link":       {nil, OpenInput{Tenant: "acme", ClientID: "ats", Token: LinkPrefix + "a-link-only-the-recipient-was-sent"}},
		"wrong prefix":       {liveShare("rudi"), OpenInput{Tenant: "acme", ClientID: "ats", Token: "anb_rt_not-a-link"}},
		"wrong tenant":       {liveShare("rudi"), OpenInput{Tenant: "other", ClientID: "ats", Token: LinkPrefix + "a-link-only-the-recipient-was-sent"}},
		"unknown app":        {liveShare("rudi"), OpenInput{Tenant: "acme", ClientID: "nope", Token: LinkPrefix + "a-link-only-the-recipient-was-sent"}},
		"disabled recipient": {liveShare("gone"), OpenInput{Tenant: "acme", ClientID: "ats", Token: LinkPrefix + "a-link-only-the-recipient-was-sent"}},
	}
	for name, c := range cases {
		uc, _, shares, _, _ := probe(t, c.share)
		_, err := uc.OpenAccessLink(context.Background(), c.in)
		if !errors.Is(err, apperr.ErrTokenInvalid) {
			t.Errorf("%s: want invalid_token, got %v", name, err)
		}
		if len(shares.touched) != 0 {
			t.Errorf("%s: a refused open was counted", name)
		}
	}
}
