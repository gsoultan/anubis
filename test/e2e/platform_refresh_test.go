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
