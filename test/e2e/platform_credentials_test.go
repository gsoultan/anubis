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

/*
How the PLATFORM authenticates its operators: the two credentials that are not
a password typed into the console -- a machine key, and the refresh family
behind a session. They were two files; they are one concept, and the folder
holds ten.
*/

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

func platformAuth() anubisv1connect.PlatformAuthServiceClient {
	return anubisv1connect.NewPlatformAuthServiceClient(http.DefaultClient, baseURL)
}

// platformSignIn performs a full login and returns the pair. Not the cached
// helper: these tests need the refresh token, and each wants its own family.
func platformSignIn(t *testing.T) *anubisv1.PlatformLoginResponse {
	t.Helper()
	resp, err := retryRateLimited(t, func() (*connect.Response[anubisv1.PlatformLoginResponse], error) {
		return platformAuth().PlatformLogin(context.Background(),
			connect.NewRequest(&anubisv1.PlatformLoginRequest{
				Username: platformUser, Password: password,
			}))
	})
	if err != nil {
		t.Fatalf("platform login: %v", err)
	}
	if resp.Msg.MfaToken != "" {
		t.Fatal("dev platform owner has MFA enrolled; scripts/db.sh devadmin resets it")
	}
	if resp.Msg.RefreshToken == "" {
		t.Fatal("login issued no refresh token: the console is back to hourly passwords")
	}
	return resp.Msg
}

// The lifecycle: rotate, then prove the THEFT property — a consumed token
// presented again kills the entire family, including the successor the
// legitimate holder is still using. Anything weaker leaves the attacker
// and the victim politely sharing a session.
func TestPlatformRefreshRotationAndTheft(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	first := platformSignIn(t)

	// Legitimate rotation works and returns a DIFFERENT pair.
	rotated, err := platformAuth().PlatformRefresh(ctx,
		connect.NewRequest(&anubisv1.PlatformRefreshRequest{RefreshToken: first.RefreshToken}))
	if err != nil {
		t.Fatalf("rotation failed: %v", err)
	}
	if rotated.Msg.RefreshToken == "" || rotated.Msg.RefreshToken == first.RefreshToken {
		t.Fatal("rotation did not mint a successor")
	}
	if rotated.Msg.AccessToken == "" {
		t.Fatal("rotation minted no access token")
	}

	// Replaying the CONSUMED token is theft: refused, and the family dies.
	if _, err := platformAuth().PlatformRefresh(ctx,
		connect.NewRequest(&anubisv1.PlatformRefreshRequest{RefreshToken: first.RefreshToken})); err == nil {
		t.Fatal("a consumed refresh token was accepted twice")
	}

	// The successor must be dead too — revocation is by family, and it must
	// have committed even though the replay itself was refused.
	if _, err := platformAuth().PlatformRefresh(ctx,
		connect.NewRequest(&anubisv1.PlatformRefreshRequest{RefreshToken: rotated.Msg.RefreshToken})); err == nil {
		t.Fatal("the successor survived reuse detection: attacker and victim share a session")
	}
}

func TestPlatformLogoutEndsTheFamily(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	sess := platformSignIn(t)

	if _, err := platformAuth().PlatformLogout(ctx,
		connect.NewRequest(&anubisv1.PlatformLogoutRequest{RefreshToken: sess.RefreshToken})); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := platformAuth().PlatformRefresh(ctx,
		connect.NewRequest(&anubisv1.PlatformRefreshRequest{RefreshToken: sess.RefreshToken})); err == nil {
		t.Fatal("a signed-out session still rotates")
	}
	// Logging out twice is fine — sign-out is not an oracle.
	if _, err := platformAuth().PlatformLogout(ctx,
		connect.NewRequest(&anubisv1.PlatformLogoutRequest{RefreshToken: sess.RefreshToken})); err != nil {
		t.Fatalf("repeated logout errored: %v", err)
	}
}
