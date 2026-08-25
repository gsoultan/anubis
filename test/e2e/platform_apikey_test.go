//go:build integration

package e2e

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
)

func platformAdmin() anubisv1connect.PlatformAdminServiceClient {
	return anubisv1connect.NewPlatformAdminServiceClient(http.DefaultClient, baseURL)
}

// apiKeyBearer presents a machine credential. Same header as a token: the
// interceptor tells them apart by prefix, not by who is asking.
func apiKeyBearer[T any](req *connect.Request[T], key string) *connect.Request[T] {
	req.Header().Set("Authorization", "Bearer "+key)
	req.Header().Set("X-Anubis-Tenant", tenant)
	return req
}

// Migration 0029 made administration operator-only, which killed manifest
// apply from CI. This proves the replacement works AND that it is not a way
// round anything: the key carries exactly its owner's authority, and dies
// with it.
func TestPlatformAPIKeyAdministersAsItsOwner(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	opToken := platformLogin(t)

	created, err := platformAdmin().CreatePlatformApiKey(ctx,
		operatorBearer(connect.NewRequest(&anubisv1.CreatePlatformApiKeyRequest{
			Label: "e2e pipeline", ExpiresInDays: 7,
		}), opToken))
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	key := created.Msg.ApiKey
	if key == "" {
		t.Fatal("no key returned: it is shown once or never")
	}
	if created.Msg.Key.GetExpiresAt() == 0 {
		t.Fatal("key has no expiry: an installation credential must not be open-ended")
	}

	// The key administers: this is the CI path that 0029 removed.
	if _, err := pageClient().ListAuthPages(ctx,
		apiKeyBearer(connect.NewRequest(&anubisv1.ListAuthPagesRequest{Kind: "signin"}), key)); err != nil {
		t.Fatalf("api key could not administer: %v", err)
	}

	// It is listed, with its public half only — never the secret.
	list, err := platformAdmin().ListPlatformApiKeys(ctx,
		operatorBearer(connect.NewRequest(&anubisv1.ListPlatformApiKeysRequest{}), opToken))
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	var found *anubisv1.PlatformApiKey
	for _, k := range list.Msg.Keys {
		if k.Id == created.Msg.Key.Id {
			found = k
		}
	}
	if found == nil {
		t.Fatal("created key is not listed")
	}
	if found.Lookup == key {
		t.Fatal("the listing exposes the whole key, not just its lookup half")
	}

	// Revocation is immediate: the next request fails, not the next hour's.
	if _, err := platformAdmin().RevokePlatformApiKey(ctx,
		operatorBearer(connect.NewRequest(&anubisv1.RevokePlatformApiKeyRequest{
			Id: created.Msg.Key.Id,
		}), opToken)); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := pageClient().ListAuthPages(ctx,
		apiKeyBearer(connect.NewRequest(&anubisv1.ListAuthPagesRequest{Kind: "signin"}), key)); err == nil {
		t.Fatal("a revoked key still administers")
	}
}

// A key is not a way past the tenant plane's own rules: it authenticates on
// the ADMIN plane as an operator, and a garbled one authenticates as nobody.
func TestPlatformAPIKeyRejectsGarbage(t *testing.T) {
	requireServer(t)
	for _, bad := range []string{
		"anb_live_deadbeef_notarealsecret",
		"anb_live_", "anb_live_x", "not-a-key-at-all",
	} {
		if _, err := pageClient().ListAuthPages(context.Background(),
			apiKeyBearer(connect.NewRequest(&anubisv1.ListAuthPagesRequest{Kind: "signin"}), bad)); err == nil {
			t.Fatalf("%q was accepted as a credential", bad)
		}
	}
}
