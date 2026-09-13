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
	tenant   = "impack"
	admin    = "admin"
	password = "anubis-dev-password"
	// The dev platform owner (scripts/db.sh devadmin). Administration is
	// operator-only since migration 0029, so admin-plane calls sign in here.
	platformUser = "devadmin"
)

// baseURL points at the dev server by default; CI starts its own instance on
// a throwaway port and overrides it.
var baseURL = func() string {
	if v := os.Getenv("ANUBIS_E2E_BASE_URL"); v != "" {
		return v
	}
	return "http://localhost:7448"
}()

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
	var resp *connect.Response[anubisv1.LoginResponse]
	var err error
	// The per-IP limiter refills at 30/min; a prior run's hammer test may
	// have drained it. Waiting for refill IS the correct behaviour under
	// test — only a non-ratelimit failure is fatal immediately.
	deadline := time.Now().Add(90 * time.Second)
	for {
		resp, err = authClient().Login(context.Background(), connect.NewRequest(&anubisv1.LoginRequest{
			Tenant: tenant, Username: admin, Password: password, ClientId: "",
		}))
		if connect.CodeOf(err) != connect.CodeResourceExhausted || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Second)
	}
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

// operatorBearer carries a PLATFORM token plus the tenant the operator is
// working in — guard.requirePlatform reads X-Anubis-Tenant on every call.
func operatorBearer[T any](req *connect.Request[T], token string) *connect.Request[T] {
	req.Header().Set("Authorization", "Bearer "+token)
	req.Header().Set("X-Anubis-Tenant", tenant)
	return req
}

// platformLogin signs in the dev platform owner. The token is cached for the
// whole run: PlatformLogin allows 5/min per account — tighter than tenant
// sign-in on purpose — and a fresh login per test would drain that budget.
var cachedPlatformToken string

func platformLogin(t *testing.T) string {
	t.Helper()
	if cachedPlatformToken != "" {
		return cachedPlatformToken
	}
	pc := anubisv1connect.NewPlatformAuthServiceClient(http.DefaultClient, baseURL)
	deadline := time.Now().Add(90 * time.Second)
	for {
		resp, err := pc.PlatformLogin(context.Background(),
			connect.NewRequest(&anubisv1.PlatformLoginRequest{
				Username: platformUser, Password: password,
			}))
		if connect.CodeOf(err) == connect.CodeResourceExhausted && time.Now().Before(deadline) {
			time.Sleep(5 * time.Second)
			continue
		}
		if err != nil {
			t.Fatalf("platform login: %v", err)
		}
		if resp.Msg.MfaToken != "" {
			t.Fatal("dev platform owner has MFA enrolled; scripts/db.sh devadmin resets it")
		}
		cachedPlatformToken = resp.Msg.AccessToken
		return cachedPlatformToken
	}
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

// TestAuthorizeThroughEngine drives the decision API with a catalog the test
// provisions itself: the anubis:* catalog died with migration 0029, so the
// suite's operator publishes a manifest, grants its role to the shared tenant
// person, and then the PERSON asks about their own access.
func TestAuthorizeThroughEngine(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	opToken := platformLogin(t)

	// Register the application (the permission key is namespaced by its
	// slug, so the app must exist first) and its catalog. Both re-runnable.
	if _, err := pageClient().CreateApplication(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateApplicationRequest{
		Application: &anubisv1.Application{Slug: "e2e-authz", Name: "e2e authz probe", Kind: "service"},
	}), opToken)); err != nil && connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("create application: %v", err)
	}
	aza := anubisv1connect.NewAuthzAdminServiceClient(http.DefaultClient, baseURL)
	// Manifest roles reference "resource:action", NOT the app-prefixed key,
	// and the role lands as "e2e-authz.reader".
	if _, err := aza.ApplyManifest(ctx, operatorBearer(connect.NewRequest(&anubisv1.ApplyManifestRequest{
		ApplicationSlug: "e2e-authz",
		ManifestJson: `{"permissions":[{"resource":"probe","action":"read","description":"e2e probe"}],
		                "roles":[{"name":"reader","description":"e2e reader","permissions":["probe:read"]}]}`,
	}), opToken)); err != nil {
		t.Fatalf("apply manifest: %v", err)
	}
	roles, err := aza.ListRoles(ctx, operatorBearer(connect.NewRequest(&anubisv1.ListRolesRequest{
		Query: "e2e-authz.reader",
	}), opToken))
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	var roleID string
	for _, r := range roles.Msg.Roles {
		if r.Name == "e2e-authz.reader" {
			roleID = r.Id
		}
	}
	if roleID == "" {
		t.Fatal("manifest role e2e-authz.reader not found after apply")
	}

	tokens := login(t)
	sc := anubisv1connect.NewSessionServiceClient(http.DefaultClient, baseURL)
	me, err := sc.GetMe(ctx, bearer(connect.NewRequest(&anubisv1.GetMeRequest{}), tokens.AccessToken))
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if _, err := aza.CreateGrant(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateGrantRequest{
		IdentityId: me.Msg.IdentityId, RoleId: roleID, Reason: "e2e authorize probe",
	}), opToken)); err != nil && connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("create grant: %v", err)
	}

	az := anubisv1connect.NewAuthzServiceClient(http.DefaultClient, baseURL)
	allow, err := az.Authorize(ctx, bearer(connect.NewRequest(&anubisv1.AuthorizeRequest{
		Subject: me.Msg.IdentityId, Permission: "e2e-authz:probe:read",
	}), tokens.AccessToken))
	if err != nil || !allow.Msg.Allow {
		t.Fatalf("granted permission must allow: %v %+v", err, allow)
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
		Subject: me.Msg.IdentityId, Permission: "e2e-authz:probe:read",
	}), tokens.AccessToken))
	if err != nil || !exp.Msg.Allow || exp.Msg.DetailJson == "" {
		t.Fatalf("explain: %v %+v", err, exp)
	}

	// --- retiring a role -----------------------------------------------------
	//
	// The identity above holds a grant of e2e-authz.reader, which is exactly
	// the state that makes this worth asserting: a manifest that stops naming
	// a role must retire it WITHOUT taking access away from anybody who
	// already has it. Deprecated is not revoked, and the two are one word
	// apart (migration 0044).
	applyManifest := func(t *testing.T, manifest string) error {
		t.Helper()
		_, err := aza.ApplyManifest(ctx, operatorBearer(connect.NewRequest(&anubisv1.ApplyManifestRequest{
			ApplicationSlug: "e2e-authz", ManifestJson: manifest,
		}), opToken))
		return err
	}
	readerState := func(t *testing.T) *anubisv1.Role {
		t.Helper()
		resp, lerr := aza.ListRoles(ctx, operatorBearer(connect.NewRequest(&anubisv1.ListRolesRequest{
			Query: "e2e-authz.reader",
		}), opToken))
		if lerr != nil {
			t.Fatalf("list roles: %v", lerr)
		}
		for _, r := range resp.Msg.Roles {
			if r.Name == "e2e-authz.reader" {
				return r
			}
		}
		t.Fatal("e2e-authz.reader vanished — retiring a role must never delete it")
		return nil
	}

	// A roles section that is present and empty would retire the lot. Refused
	// for the same reason an empty permissions section is: that is what a
	// broken export looks like, not a decision.
	if err := applyManifest(t, `{"permissions":[{"resource":"probe","action":"read"}],"roles":[]}`); err == nil {
		t.Fatal("an empty roles section wiped the catalog instead of being refused")
	}
	if readerState(t).Deprecated {
		t.Fatal("the refused apply retired a role anyway — the transaction did not roll back")
	}

	// Now a document that legitimately stops naming it.
	if err := applyManifest(t, `{"permissions":[{"resource":"probe","action":"read","description":"e2e probe"}],
	                             "roles":[{"name":"writer","description":"e2e writer","permissions":["probe:read"]}]}`); err != nil {
		t.Fatalf("apply manifest without reader: %v", err)
	}
	if !readerState(t).Deprecated {
		t.Fatal("a role the manifest stopped naming is still live")
	}

	// The whole point: the grant made before the retirement still decides.
	still, err := az.Authorize(ctx, bearer(connect.NewRequest(&anubisv1.AuthorizeRequest{
		Subject: me.Msg.IdentityId, Permission: "e2e-authz:probe:read",
	}), tokens.AccessToken))
	if err != nil || !still.Msg.Allow {
		t.Fatalf("retiring a role revoked an existing grant: %v %+v", err, still)
	}

	// What does stop is granting it to anybody new — enforced by a constraint
	// trigger, so it holds for every writer, not just this one.
	_, err = aza.CreateGrant(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateGrantRequest{
		IdentityId: me.Msg.IdentityId, RoleId: roleID, Reason: "should be refused",
	}), opToken))
	if err == nil {
		t.Fatal("a retired role was granted anew")
	}

	// Re-declaring it brings it back, or a manifest could only ever lose roles.
	if err := applyManifest(t, `{"permissions":[{"resource":"probe","action":"read","description":"e2e probe"}],
	                             "roles":[{"name":"reader","description":"e2e reader, revived","permissions":["probe:read"]}]}`); err != nil {
		t.Fatalf("re-apply with reader: %v", err)
	}
	revived := readerState(t)
	if revived.Deprecated {
		t.Fatal("re-declaring a role did not revive it")
	}
	// The same upsert has to carry the document's words through, or a synced
	// description would be silently ignored.
	if revived.Description != "e2e reader, revived" {
		t.Fatalf("description = %q, want the manifest's", revived.Description)
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

// retryLimited runs an RPC until the rate limiter stops refusing it.
//
// Several endpoints are deliberately limited — login, enrolment confirmation,
// registration — and by the time any one test runs, the suite has spent that
// budget on the tests before it. A 429 there is the limiter working, so
// treating it as a failure makes the suite report a working defence as a bug.
func retryLimited[T any](t *testing.T, what string, call func() (T, error)) T {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for {
		out, err := call()
		if connect.CodeOf(err) == connect.CodeResourceExhausted && time.Now().Before(deadline) {
			time.Sleep(2 * time.Second)
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		return out
	}
}

// signIn is Login with the rate limiter respected.
//
// The suite signs in many times before reaching any one test, and the per-IP
// login limit is real: a test that treats a 429 as a failure is reporting the
// limiter working as a bug. platformLogin already does this; anything driving
// the identity plane through several logins needs it too.
func signIn(t *testing.T, req *anubisv1.LoginRequest) *anubisv1.LoginResponse {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for {
		resp, err := authClient().Login(context.Background(), connect.NewRequest(req))
		if connect.CodeOf(err) == connect.CodeResourceExhausted && time.Now().Before(deadline) {
			time.Sleep(2 * time.Second)
			continue
		}
		if err != nil {
			t.Fatalf("login %s: %v", req.Username, err)
		}
		return resp.Msg
	}
}
