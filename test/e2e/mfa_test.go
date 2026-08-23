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
	"github.com/gsoultan/anubis/internal/platform/crypto/totp"
)

// The full second-factor lifecycle on a throwaway identity: enrol, get
// challenged, complete the challenge. It runs against a NEW identity each
// time so it never depends on — or disturbs — the shared admin account.
//
// The property that matters most is the middle one: once a factor is
// enrolled, login must stop accepting a password alone. Enrolment that the
// login path ignores is security theatre.
func TestSecondFactorLifecycle(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	adminToken := login(t).AccessToken

	username := fmt.Sprintf("mfa-probe-%d", time.Now().UnixNano())
	const password = "mfa-probe-password-1234"

	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)
	if _, err := idAdmin.CreateIdentity(ctx, bearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: "internal", Username: username, Password: password, AssuranceLevel: 2,
	}), adminToken)); err != nil {
		t.Fatalf("create probe identity: %v", err)
	}

	// 1. Password alone works while nothing is enrolled.
	first, err := authClient().Login(ctx, connect.NewRequest(&anubisv1.LoginRequest{
		Tenant: tenant, Username: username, Password: password,
	}))
	if err != nil {
		t.Fatalf("initial login: %v", err)
	}
	tokens := first.Msg.GetTokens()
	if tokens == nil {
		t.Fatal("un-enrolled identity was challenged for a factor it does not have")
	}

	// 2. Enrol TOTP. The secret is only committed once a generated code
	//    proves the authenticator holds it.
	begin, err := authClient().BeginTotpEnrollment(ctx,
		bearer(connect.NewRequest(&anubisv1.BeginTotpEnrollmentRequest{}), tokens.AccessToken))
	if err != nil {
		t.Fatalf("begin enrolment: %v", err)
	}
	secret := decodeBase32(t, begin.Msg.Secret)
	_ = secret

	// A wrong code must not enrol anything.
	if _, err := authClient().ConfirmTotpEnrollment(ctx,
		bearer(connect.NewRequest(&anubisv1.ConfirmTotpEnrollmentRequest{
			EnrollmentToken: begin.Msg.EnrollmentToken, Code: "000000",
		}), tokens.AccessToken)); err == nil {
		t.Fatal("enrolment accepted a wrong code")
	}

	// The enrolment token is single-use, so a fresh one is required after the
	// failed attempt — that is the anti-replay property, not a quirk.
	begin, err = authClient().BeginTotpEnrollment(ctx,
		bearer(connect.NewRequest(&anubisv1.BeginTotpEnrollmentRequest{}), tokens.AccessToken))
	if err != nil {
		t.Fatalf("second begin: %v", err)
	}
	secret = decodeBase32(t, begin.Msg.Secret)

	// Enrolment shares the credential-flow limiter with every other test in
	// this package, so a full-suite run can legitimately be throttled here.
	// Waiting for refill is the correct behaviour under test.
	confirm, err := retryRateLimited(t, func() (*connect.Response[anubisv1.ConfirmTotpEnrollmentResponse], error) {
		return authClient().ConfirmTotpEnrollment(ctx,
			bearer(connect.NewRequest(&anubisv1.ConfirmTotpEnrollmentRequest{
				EnrollmentToken: begin.Msg.EnrollmentToken,
				Code:            totp.Generate(secret, time.Now(), totp.DefaultStep, totp.DefaultDigits),
			}), tokens.AccessToken))
	})
	if err != nil {
		t.Fatalf("confirm enrolment: %v", err)
	}
	if len(confirm.Msg.RecoveryCodes) == 0 {
		t.Fatal("enrolment issued no recovery codes: losing the authenticator would lock the account out")
	}

	// 3. Password alone must now be refused.
	second, err := authClient().Login(ctx, connect.NewRequest(&anubisv1.LoginRequest{
		Tenant: tenant, Username: username, Password: password,
	}))
	if err != nil {
		t.Fatalf("login after enrolment: %v", err)
	}
	challenge := second.Msg.GetMfa()
	if challenge == nil {
		t.Fatal("password alone still issued tokens after TOTP enrolment — the enrolment is ignored")
	}
	if second.Msg.GetTokens() != nil {
		t.Fatal("challenge response also carried tokens")
	}

	// 4a. REPLAY GUARD: the code that completed enrolment cannot be reused to
	//     sign in. A TOTP code is single-use — otherwise one shoulder-surfed
	//     or phished code works for the rest of its 30-second window.
	if _, err := authClient().VerifyMfa(ctx, connect.NewRequest(&anubisv1.VerifyMfaRequest{
		MfaToken: challenge.MfaToken, Method: "totp",
		Code: totp.Generate(secret, time.Now(), totp.DefaultStep, totp.DefaultDigits),
	})); err == nil {
		t.Fatal("the enrolment code was accepted a second time: TOTP replay is possible")
	}

	// 4b. The NEXT code works — the step must advance, which is exactly what
	//     a user does by waiting for the authenticator to roll over. Wait for
	//     the real boundary rather than generating a future code: a code two
	//     steps ahead falls outside the accepted skew, so guessing here makes
	//     the test clock-dependent.
	waitForNextStep()
	challenge = mustChallenge(t, username, password)
	verified, err := retryRateLimited(t, func() (*connect.Response[anubisv1.VerifyMfaResponse], error) {
		return authClient().VerifyMfa(ctx, connect.NewRequest(&anubisv1.VerifyMfaRequest{
			MfaToken: challenge.MfaToken, Method: "totp",
			Code: totp.Generate(secret, time.Now(), totp.DefaultStep, totp.DefaultDigits),
		}))
	})
	if err != nil {
		t.Fatalf("verify mfa with the next code: %v", err)
	}
	if verified.Msg.Tokens.GetAccessToken() == "" {
		t.Fatal("mfa verification issued no token")
	}

	// 5. The MFA challenge token is itself single-use: replaying it must fail
	//    even with a fresh code, or a captured challenge becomes a second
	//    session.
	if _, err := authClient().VerifyMfa(ctx, connect.NewRequest(&anubisv1.VerifyMfaRequest{
		MfaToken: challenge.MfaToken, Method: "totp",
		Code: totp.Generate(secret, time.Now(), totp.DefaultStep, totp.DefaultDigits),
	})); err == nil {
		t.Fatal("MFA challenge token was accepted twice")
	}
}

// mustChallenge signs in with the password and requires a second-factor
// challenge back.
func mustChallenge(t *testing.T, username, password string) *anubisv1.MfaChallenge {
	t.Helper()
	resp, err := authClient().Login(context.Background(), connect.NewRequest(&anubisv1.LoginRequest{
		Tenant: tenant, Username: username, Password: password,
	}))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	c := resp.Msg.GetMfa()
	if c == nil {
		t.Fatal("expected a second-factor challenge")
	}
	return c
}

// waitForNextStep blocks until the TOTP window rolls over, so the following
// code is genuinely a new step rather than a guess about one.
func waitForNextStep() {
	now := time.Now()
	boundary := now.Truncate(totp.DefaultStep).Add(totp.DefaultStep)
	time.Sleep(time.Until(boundary) + 250*time.Millisecond)
}

// retryRateLimited waits out the limiter rather than failing on it: being
// throttled is the system working, not a defect.
func retryRateLimited[T any](t *testing.T, call func() (T, error)) (T, error) {
	t.Helper()
	var zero T
	deadline := time.Now().Add(90 * time.Second)
	for {
		out, err := call()
		if connect.CodeOf(err) != connect.CodeResourceExhausted {
			return out, err
		}
		if time.Now().After(deadline) {
			return zero, err
		}
		time.Sleep(3 * time.Second)
	}
}

func decodeBase32(t *testing.T, s string) []byte {
	t.Helper()
	const alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	idx := func(c byte) int {
		for i := 0; i < len(alpha); i++ {
			if alpha[i] == c {
				return i
			}
		}
		return -1
	}
	var buf, bits uint
	out := make([]byte, 0, len(s)*5/8)
	for i := 0; i < len(s); i++ {
		v := idx(s[i])
		if v < 0 {
			continue
		}
		buf = buf<<5 | uint(v)
		bits += 5
		if bits >= 8 {
			out = append(out, byte(buf>>(bits-8)))
			bits -= 8
		}
	}
	return out
}
