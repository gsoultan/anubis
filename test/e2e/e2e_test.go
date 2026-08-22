//go:build integration

// Package e2e drives a live anubisd (started by TestMain against the dev
// database) through the flows the smoke tests proved by hand. Run with:
//
//	ANUBIS_DB_URL=postgres://anubis:anubis@localhost:7449/anubis?sslmode=disable \
//	  go test -tags integration ./test/e2e/
package e2e

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
)

const (
	baseURL  = "http://localhost:7448"
	tenant   = "impack"
	admin    = "admin"
	password = "anubis-dev-password"
)

func requireServer(t *testing.T) {
	t.Helper()
	if os.Getenv("ANUBIS_DB_URL") == "" {
		t.Skip("ANUBIS_DB_URL not set")
	}
	resp, err := http.Get(baseURL + "/healthz")
	if err != nil || resp.StatusCode != 200 {
		t.Skip("anubisd not running on :7448 (scripts/api.sh)")
	}
	resp.Body.Close()
}

func authClient() anubisv1connect.AuthServiceClient {
	return anubisv1connect.NewAuthServiceClient(http.DefaultClient, baseURL)
}

func login(t *testing.T) *anubisv1.TokenPair {
	t.Helper()
	resp, err := authClient().Login(context.Background(), connect.NewRequest(&anubisv1.LoginRequest{
		Tenant: tenant, Username: admin, Password: password, ClientId: "console",
	}))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	tokens := resp.Msg.GetTokens()
	if tokens == nil {
		t.Fatalf("login answered MFA challenge unexpectedly")
	}
	return tokens
}

func bearer[T any](req *connect.Request[T], token string) *connect.Request[T] {
	req.Header().Set("Authorization", "Bearer "+token)
	return req
}

func TestLoginIssuesVerifiableToken(t *testing.T) {
	requireServer(t)
	tokens := login(t)
	if tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.ExpiresIn <= 0 {
		t.Fatalf("incomplete pair: %+v", tokens)
	}
	// Wrong password must NOT distinguish unknown user from bad password.
	_, err := authClient().Login(context.Background(), connect.NewRequest(&anubisv1.LoginRequest{
		Tenant: tenant, Username: admin, Password: "wrong-password-here",
	}))
	badPass := connect.CodeOf(err)
	_, err = authClient().Login(context.Background(), connect.NewRequest(&anubisv1.LoginRequest{
		Tenant: tenant, Username: "no-such-user-ever", Password: "wrong-password-here",
	}))
	if connect.CodeOf(err) != badPass {
		t.Fatalf("unknown-user and wrong-password answered differently: %v vs %v",
			connect.CodeOf(err), badPass)
	}
}

// TestRefreshTheftDetection is the regression test for the rollback bug the
// live smoke found: after reuse trips the alarm, the SUCCESSOR must be dead
// too — the revocation has to survive the failed claim's transaction.
func TestRefreshTheftDetection(t *testing.T) {
	requireServer(t)
	tokens := login(t)
	ctx := context.Background()

	r1, err := authClient().Refresh(ctx, connect.NewRequest(&anubisv1.RefreshRequest{
		RefreshToken: tokens.RefreshToken,
	}))
	if err != nil {
		t.Fatalf("legit rotation failed: %v", err)
	}
	successor := r1.Msg.Tokens.RefreshToken

	// Reuse the consumed token: theft detected.
	_, err = authClient().Refresh(ctx, connect.NewRequest(&anubisv1.RefreshRequest{
		RefreshToken: tokens.RefreshToken,
	}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("reuse: want unauthenticated, got %v", err)
	}

	// The successor must ALSO be dead (family revoked, durably).
	_, err = authClient().Refresh(ctx, connect.NewRequest(&anubisv1.RefreshRequest{
		RefreshToken: successor,
	}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("successor survived family revocation: %v", err)
	}
}

func TestAuthorizeThroughEngine(t *testing.T) {
	requireServer(t)
	tokens := login(t)
	ctx := context.Background()
	sc := anubisv1connect.NewSessionServiceClient(http.DefaultClient, baseURL)
	me, err := sc.GetMe(ctx, bearer(connect.NewRequest(&anubisv1.GetMeRequest{}), tokens.AccessToken))
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	az := anubisv1connect.NewAuthzServiceClient(http.DefaultClient, baseURL)

	allow, err := az.Authorize(ctx, bearer(connect.NewRequest(&anubisv1.AuthorizeRequest{
		Subject: me.Msg.IdentityId, Permission: "anubis:identity:read",
	}), tokens.AccessToken))
	if err != nil || !allow.Msg.Allow {
		t.Fatalf("admin must hold anubis:identity:read: %v %+v", err, allow)
	}

	deny, err := az.Authorize(ctx, bearer(connect.NewRequest(&anubisv1.AuthorizeRequest{
		Subject: me.Msg.IdentityId, Permission: "nonexistent:thing:do",
	}), tokens.AccessToken))
	if err != nil || deny.Msg.Allow {
		t.Fatalf("unknown permission must deny: %v %+v", err, deny)
	}
	if deny.Msg.Reason == "" {
		t.Fatal("deny must carry a machine-readable reason")
	}

	exp, err := az.Explain(ctx, bearer(connect.NewRequest(&anubisv1.ExplainRequest{
		Subject: me.Msg.IdentityId, Permission: "anubis:identity:read",
	}), tokens.AccessToken))
	if err != nil || !exp.Msg.Allow || exp.Msg.DetailJson == "" {
		t.Fatalf("explain: %v %+v", err, exp)
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	requireServer(t)
	tokens := login(t)
	ctx := context.Background()
	_, err := authClient().Logout(ctx, bearer(connect.NewRequest(&anubisv1.LogoutRequest{}), tokens.AccessToken))
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	// The refresh chain dies with the session.
	_, err = authClient().Refresh(ctx, connect.NewRequest(&anubisv1.RefreshRequest{
		RefreshToken: tokens.RefreshToken,
	}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("refresh after logout must fail: %v", err)
	}
}

func TestGateDecisions(t *testing.T) {
	requireServer(t)
	tokens := login(t)
	check := func(uri, authz string) int {
		req, _ := http.NewRequest("POST", baseURL+"/v1/gate/check", nil)
		req.Header.Set("X-Original-URI", uri)
		req.Header.Set("X-Original-Method", "GET")
		if authz != "" {
			req.Header.Set("Authorization", "Bearer "+authz)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("gate: %v", err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	// Depends on the billing manifest applied by the bootstrap smoke; skip
	// cleanly when absent.
	if code := check("/public/pricing", ""); code == http.StatusForbidden {
		t.Skip("billing routes not applied on this database")
	} else if code != http.StatusNoContent {
		t.Fatalf("public: want 204, got %d", code)
	}
	if code := check("/invoices/42", ""); code != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", code)
	}
	if code := check("/public/%2e%2e/invoices/42", ""); code != http.StatusForbidden {
		t.Fatalf("traversal: want 403, got %d", code)
	}
	_ = tokens
}

func TestRateLimitTripsOnHammering(t *testing.T) {
	requireServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tripped := false
	for i := 0; i < 40; i++ {
		_, err := authClient().Login(ctx, connect.NewRequest(&anubisv1.LoginRequest{
			Tenant: tenant, Username: "rate-limit-target", Password: "x-definitely-wrong",
		}))
		if connect.CodeOf(err) == connect.CodeResourceExhausted {
			tripped = true
			break
		}
	}
	if !tripped {
		t.Fatal("per-account limiter never tripped after 40 attempts")
	}
}
