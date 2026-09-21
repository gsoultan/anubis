//go:build integration

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
)

/*
How the PLATFORM authenticates its operators: the machine key, the refresh
family behind a session, and the password itself -- who may change it, and
what stops working when they do. They were separate files; they are one
concept, and the folder holds ten.
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

// newOperator creates a throwaway operator with a live assignment and returns
// their username. It never touches the shared owner account: changing that
// password would end every other test in this package.
func newOperator(t *testing.T, password string) string {
	t.Helper()
	ctx := context.Background()
	token := platformLogin(t)
	admin := platformAdmin()

	username := fmt.Sprintf("op-%d", time.Now().UnixNano())
	if _, err := admin.CreateOperator(ctx, bearer(connect.NewRequest(&anubisv1.CreateOperatorRequest{
		Username: username, Email: username + "@example.test",
		Password: password, Role: "support",
	}), token)); err != nil {
		t.Fatalf("create operator: %v", err)
	}
	return username
}

func operatorLogin(t *testing.T, username, password string) (string, error) {
	t.Helper()
	pc := platformAuth()
	deadline := time.Now().Add(90 * time.Second)
	for {
		resp, err := pc.PlatformLogin(context.Background(),
			connect.NewRequest(&anubisv1.PlatformLoginRequest{
				Username: username, Password: password,
			}))
		if connect.CodeOf(err) == connect.CodeResourceExhausted && time.Now().Before(deadline) {
			time.Sleep(3 * time.Second)
			continue
		}
		if err != nil {
			return "", err
		}
		return resp.Msg.GetAccessToken(), nil
	}
}

// An operator must be able to rotate their own password.
//
// Nothing could, until now. PlatformAuthService offered login, MFA, refresh,
// logout and TOTP enrolment; operator admin offered create, assign and
// set-status. A password written at install or at CreateOperator was
// permanent, so the remedy for a suspected compromise was to disable the
// account and build another — losing its assignments and its history.
func TestAnOperatorCanChangeTheirPassword(t *testing.T) {
	requireServer(t)
	ctx := context.Background()

	const first = "operator-first-password-1234"
	const second = "operator-second-password-5678"
	username := newOperator(t, first)

	token, err := operatorLogin(t, username, first)
	if err != nil {
		t.Fatalf("sign in with the first password: %v", err)
	}

	pc := platformAuth()
	if _, err := pc.ChangePlatformPassword(ctx, bearer(connect.NewRequest(
		&anubisv1.ChangePlatformPasswordRequest{
			CurrentPassword: first, NewPassword: second,
		}), token)); err != nil {
		t.Fatalf("change password: %v", err)
	}

	// The old password is dead.
	if _, err := operatorLogin(t, username, first); err == nil {
		t.Fatal("the old password still signs in — the rotation changed nothing")
	}
	// The new one works.
	if _, err := operatorLogin(t, username, second); err != nil {
		t.Fatalf("the new password does not sign in: %v", err)
	}

	// And the token held while changing it is dead too. Sparing the caller's
	// own session would spare an attacker's, which is the case this exists
	// for.
	if _, err := pc.MyTenants(ctx, bearer(connect.NewRequest(
		&anubisv1.MyTenantsRequest{}), token)); err == nil {
		t.Fatal("a token minted under the OLD password still works after the " +
			"rotation; every session it opened survives")
	}
}

// The current password is required, so a stolen access token cannot be turned
// into permanent ownership of the account.
func TestChangingAPasswordNeedsTheCurrentOne(t *testing.T) {
	requireServer(t)
	ctx := context.Background()

	const password = "operator-guard-password-1234"
	username := newOperator(t, password)
	token, err := operatorLogin(t, username, password)
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}

	pc := platformAuth()
	if _, err := pc.ChangePlatformPassword(ctx, bearer(connect.NewRequest(
		&anubisv1.ChangePlatformPasswordRequest{
			CurrentPassword: "not-the-current-password", NewPassword: "a-brand-new-password-9999",
		}), token)); err == nil {
		t.Fatal("an access token alone rewrote the password it could not produce")
	}
	// A password that is too short is refused before anything is written.
	if _, err := pc.ChangePlatformPassword(ctx, bearer(connect.NewRequest(
		&anubisv1.ChangePlatformPasswordRequest{
			CurrentPassword: password, NewPassword: "short",
		}), token)); err == nil {
		t.Fatal("a password below the installation floor was accepted")
	}
	// The original still works: a refused change must change nothing.
	if _, err := operatorLogin(t, username, password); err != nil {
		t.Fatalf("a refused change broke the existing password: %v", err)
	}
}

// Disabling an operator must take effect now, not when their token expires.
//
// SetPlatformUserStatus advances token_epoch and its comment says why —
// "token_epoch + 1 is what makes disabling take effect NOW rather than
// whenever the token expired". Nothing compared it. The guard checked
// assignments and never read the operator's row, so a disabled operator kept
// full authority for up to an hour: PlatformRefresh checks Active(), which
// bounds the window at one access-token TTL without closing it.
func TestDisablingAnOperatorEndsTheirSessionImmediately(t *testing.T) {
	requireServer(t)
	ctx := context.Background()

	const password = "operator-disable-password-1234"
	username := newOperator(t, password)
	token, err := operatorLogin(t, username, password)
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}
	pc := platformAuth()
	if _, err := pc.MyTenants(ctx, bearer(connect.NewRequest(
		&anubisv1.MyTenantsRequest{}), token)); err != nil {
		t.Fatalf("the token does not work before disabling; this proves nothing: %v", err)
	}

	admin := platformAdmin()
	list, err := admin.ListOperators(ctx, bearer(connect.NewRequest(
		&anubisv1.ListOperatorsRequest{Query: username, PageSize: 10}), platformLogin(t)))
	if err != nil {
		t.Fatalf("list operators: %v", err)
	}
	var id string
	for _, o := range list.Msg.Operators {
		if o.Username == username {
			id = o.IdentityId
		}
	}
	if id == "" {
		t.Fatalf("could not find the operator just created")
	}
	if _, err := admin.SetOperatorStatus(ctx, bearer(connect.NewRequest(
		&anubisv1.SetOperatorStatusRequest{OperatorId: id, Status: "disabled"}),
		platformLogin(t))); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if _, err := pc.MyTenants(ctx, bearer(connect.NewRequest(
		&anubisv1.MyTenantsRequest{}), token)); err == nil {
		t.Fatal("a disabled operator's access token still works — disabling " +
			"waits for the token to expire, up to an hour of unchanged authority")
	}
}
