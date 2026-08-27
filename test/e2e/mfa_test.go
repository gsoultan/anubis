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
	adminToken := platformLogin(t)

	username := fmt.Sprintf("mfa-probe-%d", time.Now().UnixNano())
	const password = "mfa-probe-password-1234"

	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)
	if _, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
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

// --- enrol-or-deny rollout (docs/enrolment-rollout.md) ---

// The whole enrol-or-deny arc, in the order an operator would live it.
//
// The gap this closes: a realm could list `totp` in required_factors and still
// admit a password-only login from somebody who never enrolled one. It stayed
// open deliberately, because closing it with a boolean is a lockout — the
// enrolment endpoint needs a session, and the policy withholds exactly that.
//
// So the switch is a date, and the refusal carries the means to comply. This
// walks all four states: not in force, inside the grace period, past it, and
// after the member enrols.
func TestEnrolOrDenyRollout(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	token := platformLogin(t)
	auth := authClient()
	admin := anubisv1connect.NewTenantAdminServiceClient(http.DefaultClient, baseURL)
	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)

	realmCode := fmt.Sprintf("enrol%d", time.Now().UnixNano()%1e6)
	created, err := admin.CreateRealm(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateRealmRequest{
		Realm: &anubisv1.Realm{
			Code: realmCode, Kind: "internal", DisplayName: "Enrolment probe",
			MinAssurance:    1,
			AllowedFactors:  []string{"password", "totp"},
			RequiredFactors: []string{"password", "totp"},
			SessionTtl:      "8 hours", AccessTokenTtl: "10 minutes",
			RefreshTokenTtl: "30 days",
		},
	}), token))
	if err != nil {
		t.Fatalf("create realm: %v", err)
	}
	realm := created.Msg.GetRealm()

	username := fmt.Sprintf("enrolee-%d", time.Now().UnixNano())
	const password = "enrolee-probe-password-1234"
	if _, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: realmCode, Username: username, Password: password,
	}), token)); err != nil {
		t.Fatalf("create identity: %v", err)
	}

	login := func() *anubisv1.LoginResponse {
		t.Helper()
		return signIn(t, &anubisv1.LoginRequest{
			Tenant: tenant, Realm: realmCode, Username: username, Password: password,
		})
	}
	setDeadline := func(unix int64) {
		t.Helper()
		realm.FactorEnrolmentDeadline = unix
		if _, uerr := admin.UpdateRealm(ctx, operatorBearer(connect.NewRequest(&anubisv1.UpdateRealmRequest{
			Realm: realm,
		}), token)); uerr != nil {
			t.Fatalf("set deadline %d: %v", unix, uerr)
		}
	}

	// 1. required_factors says totp, nobody has enrolled, no deadline set.
	//    This must behave exactly as it did before the feature existed, or
	//    upgrading Anubis locks out every realm that ever listed a factor.
	if got := login(); got.GetTokens() == nil {
		t.Fatalf("a realm with no deadline refused a login: %T", got.GetResult())
	} else if got.GetEnrolmentDue() != nil {
		t.Fatal("warned about a deadline that does not exist")
	}

	// 2. Inside the grace period: sign-in still works, and now says why it
	//    will not later. A grace period that refuses is not a grace period.
	deadline := time.Now().Add(48 * time.Hour)
	setDeadline(deadline.Unix())
	graced := login()
	if graced.GetTokens() == nil {
		t.Fatalf("the grace period refused a login: %T", graced.GetResult())
	}
	due := graced.GetEnrolmentDue()
	if due == nil {
		t.Fatal("signed in inside the grace period with no warning at all")
	}
	if len(due.Factors) != 1 || due.Factors[0] != "totp" {
		t.Fatalf("warned about %v, want [totp]", due.Factors)
	}
	if due.Deadline != deadline.Unix() {
		t.Fatalf("warned about %d, want %d", due.Deadline, deadline.Unix())
	}
	if due.GrantToken != "" {
		t.Fatal("a grant token was issued alongside a real session — " +
			"that is a second credential nobody asked for")
	}

	// 3. Past the deadline: no session, and a challenge that can be acted on.
	setDeadline(time.Now().Add(-1 * time.Hour).Unix())
	refused := login()
	if refused.GetTokens() != nil {
		t.Fatal("an overdue member was let in anyway")
	}
	req := refused.GetEnrolmentRequired()
	if req == nil {
		t.Fatalf("overdue login returned %T, not an enrolment challenge", refused.GetResult())
	}
	if req.GrantToken == "" {
		t.Fatal("refused without a grant token — that is deny, not enrol-or-deny")
	}

	// 4. The grant is enough to enrol with, without a session. This is the
	//    whole point: the policy withholds the session, so demanding one
	//    would make it unsatisfiable by the people it applies to.
	begun := retryLimited(t, "enrol with the grant", func() (*connect.Response[anubisv1.BeginTotpEnrollmentResponse], error) {
		return auth.BeginTotpEnrollment(ctx,
			connect.NewRequest(&anubisv1.BeginTotpEnrollmentRequest{GrantToken: req.GrantToken}))
	})
	secret := decodeBase32(t, begun.Msg.Secret)
	retryLimited(t, "confirm with the grant", func() (*connect.Response[anubisv1.ConfirmTotpEnrollmentResponse], error) {
		return auth.ConfirmTotpEnrollment(ctx,
			connect.NewRequest(&anubisv1.ConfirmTotpEnrollmentRequest{
				EnrollmentToken: begun.Msg.EnrollmentToken,
				Code:            totp.Generate(secret, time.Now(), totp.DefaultStep, totp.DefaultDigits),
				GrantToken:      req.GrantToken,
			}))
	})

	// 5. Having complied, the member signs in again — and is challenged for
	//    the factor they now hold, not refused for the one they lack.
	after := login()
	if after.GetEnrolmentRequired() != nil {
		t.Fatal("still refused after enrolling")
	}
	if after.GetMfa() == nil {
		t.Fatalf("after enrolling, login returned %T instead of an MFA challenge",
			after.GetResult())
	}
}

// A grant is minted only for a member with nothing enrolled. Once one is
// enrolled, redeeming an old grant would REPLACE the authenticator — turning
// a leaked grant into an account takeover rather than a first enrolment.
func TestAGrantCannotReplaceAnEnrolledFactor(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	token := platformLogin(t)
	auth := authClient()
	admin := anubisv1connect.NewTenantAdminServiceClient(http.DefaultClient, baseURL)
	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)

	realmCode := fmt.Sprintf("regrant%d", time.Now().UnixNano()%1e6)
	created, err := admin.CreateRealm(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateRealmRequest{
		Realm: &anubisv1.Realm{
			Code: realmCode, Kind: "internal", DisplayName: "Grant reuse probe",
			MinAssurance:    1,
			AllowedFactors:  []string{"password", "totp"},
			RequiredFactors: []string{"password", "totp"},
			SessionTtl:      "8 hours", AccessTokenTtl: "10 minutes",
			RefreshTokenTtl: "30 days",
			// Already overdue, so the first login hands out a grant.
			FactorEnrolmentDeadline: time.Now().Add(-time.Hour).Unix(),
		},
	}), token))
	if err != nil {
		t.Fatalf("create realm: %v", err)
	}
	_ = created

	username := fmt.Sprintf("regrant-%d", time.Now().UnixNano())
	const password = "regrant-probe-password-1234"
	if _, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: realmCode, Username: username, Password: password,
	}), token)); err != nil {
		t.Fatalf("create identity: %v", err)
	}

	first := signIn(t, &anubisv1.LoginRequest{
		Tenant: tenant, Realm: realmCode, Username: username, Password: password,
	})
	grant := first.GetEnrolmentRequired().GetGrantToken()
	if grant == "" {
		t.Fatal("no grant issued for an overdue member")
	}

	// Enrol once, legitimately.
	begun := retryLimited(t, "first enrolment", func() (*connect.Response[anubisv1.BeginTotpEnrollmentResponse], error) {
		return auth.BeginTotpEnrollment(ctx,
			connect.NewRequest(&anubisv1.BeginTotpEnrollmentRequest{GrantToken: grant}))
	})
	retryLimited(t, "first confirm", func() (*connect.Response[anubisv1.ConfirmTotpEnrollmentResponse], error) {
		return auth.ConfirmTotpEnrollment(ctx,
			connect.NewRequest(&anubisv1.ConfirmTotpEnrollmentRequest{
				EnrollmentToken: begun.Msg.EnrollmentToken,
				Code:            totp.Generate(decodeBase32(t, begun.Msg.Secret), time.Now(), totp.DefaultStep, totp.DefaultDigits),
				GrantToken:      grant,
			}))
	})

	// The same grant, replayed, must not start a second enrolment.
	_, err = auth.BeginTotpEnrollment(ctx,
		connect.NewRequest(&anubisv1.BeginTotpEnrollmentRequest{GrantToken: grant}))
	if connect.CodeOf(err) == connect.CodeResourceExhausted {
		t.Skip("rate limiter refused the replay probe before the server could judge it")
	}
	if err == nil {
		t.Fatal("a spent grant re-enrolled an identity that already has a factor")
	}
	if connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("want a conflict on grant replay, got %v", err)
	}
}
