package gateapp

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"github.com/gsoultan/anubis/internal/gate/snapshot"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/pkg/anubis"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

// An access-link token opens one record through the decision endpoint and
// nothing through the gate: a route policy names places, and a link must
// never become a way through the front door — not even to a route the
// recipient's own grants would pass.
func TestAnAccessLinkTokenDoesNotPassTheGate(t *testing.T) {
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
	snap := grantedSnapshot()
	snap.TenantSlug = "acme"
	snap.Routes = []snapshot.Route{{AppSlug: "ats", Effect: "require_auth", PathPattern: "/requisitions", Methods: []string{"GET"}}}
	d := NewDecider("https://id.example.test", keyring.NewManager(ring), fixedSnapshots{data: snap, fresh: true})
	now := time.Now()

	mint := func(amr []string) string {
		body, _ := json.Marshal(anubis.Claims{
			Issuer: "https://id.example.test", Subject: "dewi", Audience: []string{"ats"}, Tenant: "acme",
			Expires: now.Add(5 * time.Minute).Unix(), IssuedAt: now.Unix(), NotBefore: now.Unix(),
			TokenID: "jti", AMR: amr, AuthTime: now.Unix(), Epoch: 1, Version: 1,
			Scopes: map[string]string{"_record": "job_requisition/JR-0142"},
		})
		footer, _ := json.Marshal(map[string]string{"kid": "k1"})
		tok, err := paseto.Sign(priv, body, footer, nil)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	req := Request{Tenant: "acme", Method: "GET", Path: "/requisitions", Host: "ats.example.test"}

	req.Token = mint([]string{"pwd"})
	if got := d.Decide(req); got.Outcome != Allow {
		t.Fatalf("an ordinary token was refused on a require_auth route: %+v", got)
	}
	req.Token = mint([]string{"link"})
	if got := d.Decide(req); got.Outcome != Denied {
		t.Fatalf("a link token passed the gate: %+v", got)
	}
}
