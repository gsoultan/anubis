//go:build integration

package e2e

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
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

	realmCode := fmt.Sprintf("enrol%d", time.Now().UnixNano())
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

	realmCode := fmt.Sprintf("regrant%d", time.Now().UnixNano())
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

// A second factor is a property of the IDENTITY, not of the door it walks
// through.
//
// AuthService.Login refuses a password-only sign-in once an authenticator is
// enrolled — TestSecondFactorLifecycle asserts exactly that, and the
// interactor says why: "somebody who added an authenticator is asked for it
// either way". The hosted browser page is a second implementation of login,
// and it went password -> session -> SSO cookie -> authorization code with no
// factor check anywhere in the handler.
//
// So an attacker holding only a stolen password could skip the victim's
// enrolled TOTP by using POST /v1/login instead of the API.
func TestBrowserLoginRefusesPasswordAloneOnceEnrolled(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	adminToken := platformLogin(t)

	username := fmt.Sprintf("browser-mfa-%d", time.Now().UnixNano())
	const password = "browser-mfa-password-1234"

	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)
	if _, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: "internal", Username: username, Password: password, AssuranceLevel: 2,
	}), adminToken)); err != nil {
		t.Fatalf("create probe identity: %v", err)
	}

	// Every login in this package shares one limiter, and this test adds
	// several more. Waiting for refill is the correct behaviour under test.
	first, err := retryRateLimited(t, func() (*connect.Response[anubisv1.LoginResponse], error) {
		return authClient().Login(ctx, connect.NewRequest(&anubisv1.LoginRequest{
			Tenant: tenant, Username: username, Password: password,
		}))
	})
	if err != nil {
		t.Fatalf("initial login: %v", err)
	}
	tokens := first.Msg.GetTokens()
	if tokens == nil {
		t.Fatal("un-enrolled identity was challenged")
	}

	// Enrol TOTP through the API.
	begin, err := authClient().BeginTotpEnrollment(ctx,
		bearer(connect.NewRequest(&anubisv1.BeginTotpEnrollmentRequest{}), tokens.AccessToken))
	if err != nil {
		t.Fatalf("begin enrolment: %v", err)
	}
	secret := decodeBase32(t, begin.Msg.Secret)
	if _, err := retryRateLimited(t, func() (*connect.Response[anubisv1.ConfirmTotpEnrollmentResponse], error) {
		return authClient().ConfirmTotpEnrollment(ctx,
			bearer(connect.NewRequest(&anubisv1.ConfirmTotpEnrollmentRequest{
				EnrollmentToken: begin.Msg.EnrollmentToken,
				Code:            totp.Generate(secret, time.Now(), totp.DefaultStep, totp.DefaultDigits),
			}), tokens.AccessToken))
	}); err != nil {
		t.Fatalf("confirm enrolment: %v", err)
	}

	// The API now refuses password alone. Establishes that the identity is
	// genuinely enrolled, so a pass below cannot be "TOTP was never on".
	after, err := retryRateLimited(t, func() (*connect.Response[anubisv1.LoginResponse], error) {
		return authClient().Login(ctx, connect.NewRequest(&anubisv1.LoginRequest{
			Tenant: tenant, Username: username, Password: password,
		}))
	})
	if err != nil {
		t.Fatalf("login after enrolment: %v", err)
	}
	if after.Msg.GetMfa() == nil {
		t.Fatal("the API stopped demanding the enrolled factor; this test proves nothing")
	}

	// THE BROWSER DOOR. Same credentials, same identity, no second factor.
	// Through a rendered page, as a browser does: the form carries a CSRF
	// token and a cross-site submission has none.
	client, csrf := signinPage(t)
	form := url.Values{
		"tenant": {tenant}, "realm": {"internal"},
		"username": {username}, "password": {password},
		"csrf": {csrf},
	}
	resp := postLoginForm(t, client, form)
	defer resp.Body.Close()

	for _, c := range resp.Cookies() {
		if strings.Contains(c.Name, "anubis_sso") && c.Value != "" {
			t.Fatalf("the browser door issued an SSO session cookie for a "+
				"password-only login on an identity with TOTP enrolled "+
				"(cookie %q, status %d)", c.Name, resp.StatusCode)
		}
	}
	if loc := resp.Header.Get("Location"); strings.Contains(loc, "code=") {
		t.Fatalf("the browser door issued an authorization code for a "+
			"password-only login on an identity with TOTP enrolled: %s", loc)
	}

	// And it must ASK: closing the hole by refusing MFA users outright would
	// pass the assertions above and break the product.
	body := readAll(t, resp)
	if !strings.Contains(body, `name="mfa_token"`) {
		t.Fatalf("the page did not ask for a second factor (status %d):\n%s",
			resp.StatusCode, firstBytes(body, 400))
	}
	token := between(body, `name="mfa_token" value="`, `"`)
	if token == "" {
		t.Fatal("the second-factor form carried no token")
	}

	// The enrolment code must NOT complete the browser sign-in either: a
	// code is single-use whichever door it is presented at. The browser
	// path goes through the same AdvanceCredentialStep guard.
	stale := postLoginForm(t, client, url.Values{
		"tenant": {tenant}, "realm": {"internal"},
		"username": {username}, "password": {password},
		"csrf":      {csrfField.FindStringSubmatch(body)[1]},
		"mfa_token": {token},
		"code":      {totp.Generate(secret, time.Now(), totp.DefaultStep, totp.DefaultDigits)},
	})
	staleBody := readAll(t, stale)
	for _, c := range stale.Cookies() {
		if strings.Contains(c.Name, "anubis_sso") && c.Value != "" {
			t.Fatal("the enrolment code signed in through the browser door: TOTP replay is possible")
		}
	}
	// A rejected code must leave the user able to try again, or one typo
	// means restarting the whole sign-in.
	token = between(staleBody, `name="mfa_token" value="`, `"`)
	if token == "" {
		t.Fatalf("a wrong code left no way to retry:\n%s", firstBytes(staleBody, 400))
	}

	// The NEXT code completes it — the factor is a step, not a wall. Waiting
	// for the real boundary rather than generating a future code, which
	// would fall outside the accepted skew.
	waitForNextStep()
	done := postLoginForm(t, client, url.Values{
		"tenant": {tenant}, "realm": {"internal"},
		"username": {username}, "password": {password},
		"csrf":      {csrfField.FindStringSubmatch(staleBody)[1]},
		"mfa_token": {token},
		"code":      {totp.Generate(secret, time.Now(), totp.DefaultStep, totp.DefaultDigits)},
	})
	var got string
	doneBody := readAll(t, done)
	for _, c := range done.Cookies() {
		if strings.Contains(c.Name, "anubis_sso") && c.Value != "" {
			got = c.Name
		}
	}
	if got == "" {
		t.Fatalf("a correct code did not complete the browser sign-in (status %d):\n%s",
			done.StatusCode, firstBytes(doneBody, 400))
	}
}

func between(s, open, close string) string {
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func firstBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// postLoginForm posts the hosted sign-in form, waiting out the shared rate
// limiter the same way retryRateLimited does for the RPC path. Every test in
// this package hits one limiter from one IP, so throttling here is expected
// under a full-suite run rather than a failure.
func postLoginForm(t *testing.T, client *http.Client, form url.Values) *http.Response {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for {
		resp, err := client.PostForm(baseURL+"/v1/login", form)
		if err != nil {
			t.Fatalf("browser login: %v", err)
		}
		if resp.StatusCode != http.StatusTooManyRequests || time.Now().After(deadline) {
			return resp
		}
		resp.Body.Close()
		time.Sleep(3 * time.Second)
	}
}

// A realm past its enrolment deadline refuses the API door. It must refuse
// the browser door too.
//
// TestEnrolOrDenyRollout proves the policy through AuthService.Login.
// The hosted page is the OTHER implementation of login, and it asked only
// "does this identity hold a factor?" — never "does this realm still admit
// somebody without one?". EnrolmentStanceFor had exactly one caller in the
// whole tree, in the interactor.
//
// So a realm whose deadline had passed refused every API client and signed
// the same member in through a browser, which is the door humans use. The
// policy was void for precisely the population it was written for, and the
// roadmap listed it under claims that are closed.
func TestBrowserLoginHonoursTheEnrolmentDeadline(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	token := platformLogin(t)
	admin := anubisv1connect.NewTenantAdminServiceClient(http.DefaultClient, baseURL)
	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)

	realmCode := fmt.Sprintf("benrol%d", time.Now().UnixNano()%100_000_000)
	created, err := admin.CreateRealm(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateRealmRequest{
		Realm: &anubisv1.Realm{
			Code: realmCode, Kind: "internal", DisplayName: "Browser enrolment probe",
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

	username := fmt.Sprintf("benrolee-%d", time.Now().UnixNano())
	const password = "browser-enrolee-password-1234"
	if _, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: realmCode, Username: username, Password: password,
	}), token)); err != nil {
		t.Fatalf("create identity: %v", err)
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
	browserLogin := func() (*http.Response, string) {
		t.Helper()
		client, csrf := signinPage(t)
		resp := postLoginForm(t, client, url.Values{
			"tenant": {tenant}, "realm": {realmCode},
			"username": {username}, "password": {password},
			"csrf": {csrf},
		})
		return resp, readAll(t, resp)
	}
	signedIn := func(resp *http.Response) string {
		t.Helper()
		for _, c := range resp.Cookies() {
			if strings.Contains(c.Name, "anubis_sso") && c.Value != "" {
				return c.Name
			}
		}
		return ""
	}

	// Past the deadline, with nothing enrolled.
	setDeadline(time.Now().Add(-1 * time.Hour).Unix())

	// The API refuses. Without this the browser assertions below would pass
	// just as well if the policy had never applied at all.
	refused := signIn(t, &anubisv1.LoginRequest{
		Tenant: tenant, Realm: realmCode, Username: username, Password: password,
	})
	if refused.GetEnrolmentRequired() == nil {
		t.Fatalf("the API stopped refusing an overdue member (%T); this test proves nothing",
			refused.GetResult())
	}

	// THE BROWSER DOOR. Same realm, same member, same missing factor.
	resp, body := browserLogin()
	defer resp.Body.Close()
	if c := signedIn(resp); c != "" {
		t.Fatalf("the browser door issued an SSO session cookie to a member the "+
			"realm's enrolment deadline refuses (cookie %q, status %d)", c, resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); strings.Contains(loc, "code=") {
		t.Fatalf("the browser door issued an authorization code past the "+
			"enrolment deadline: %s", loc)
	}
	// A refusal nobody can act on is a support ticket. It has to name the
	// factor that has to be enrolled.
	if !strings.Contains(strings.ToLower(body), "authenticator") {
		t.Fatalf("refused without saying what to enrol (status %d):\n%s",
			resp.StatusCode, firstBytes(body, 400))
	}

	// And the cliff must stay a DATE, not a wall. Refusing every unenrolled
	// member would satisfy every assertion above and lock out each realm that
	// ever listed a factor — the exact outcome the deadline exists to avoid.
	setDeadline(time.Now().Add(48 * time.Hour).Unix())
	graced, gracedBody := browserLogin()
	defer graced.Body.Close()
	if signedIn(graced) == "" {
		t.Fatalf("the grace period refused a browser sign-in (status %d):\n%s",
			graced.StatusCode, firstBytes(gracedBody, 400))
	}
}

// The second-factor form the server renders must be submittable by a browser.
//
// TestBrowserLoginRefusesPasswordAloneOnceEnrolled completes this flow by
// posting a username and password on the second submit. The rendered form has
// neither field — the MFA branch of the template carries `mfa_token` and a
// code, and its comment says why: "the password is not asked for again, and
// nothing on this page can replay it". So that test drives a client nobody
// ships, and passes against a form no human could submit.
//
// LoginForm required a valid password on EVERY submit, including the one the
// token was supposed to stand for. A real browser therefore got "Invalid
// username or password" after typing a correct code, and the hosted
// second-factor step could not be completed at all.
//
// This test posts exactly the fields the page contains, which is the only
// thing a browser can do.
func TestTheRenderedSecondFactorFormCanBeSubmitted(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	adminToken := platformLogin(t)

	username := fmt.Sprintf("form-mfa-%d", time.Now().UnixNano())
	const password = "form-mfa-password-1234"

	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)
	if _, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: "internal", Username: username, Password: password, AssuranceLevel: 2,
	}), adminToken)); err != nil {
		t.Fatalf("create probe identity: %v", err)
	}

	first, err := retryRateLimited(t, func() (*connect.Response[anubisv1.LoginResponse], error) {
		return authClient().Login(ctx, connect.NewRequest(&anubisv1.LoginRequest{
			Tenant: tenant, Username: username, Password: password,
		}))
	})
	if err != nil {
		t.Fatalf("initial login: %v", err)
	}
	tokens := first.Msg.GetTokens()
	if tokens == nil {
		t.Fatal("un-enrolled identity was challenged")
	}
	begin, err := authClient().BeginTotpEnrollment(ctx,
		bearer(connect.NewRequest(&anubisv1.BeginTotpEnrollmentRequest{}), tokens.AccessToken))
	if err != nil {
		t.Fatalf("begin enrolment: %v", err)
	}
	secret := decodeBase32(t, begin.Msg.Secret)
	if _, err := retryRateLimited(t, func() (*connect.Response[anubisv1.ConfirmTotpEnrollmentResponse], error) {
		return authClient().ConfirmTotpEnrollment(ctx,
			bearer(connect.NewRequest(&anubisv1.ConfirmTotpEnrollmentRequest{
				EnrollmentToken: begin.Msg.EnrollmentToken,
				Code:            totp.Generate(secret, time.Now(), totp.DefaultStep, totp.DefaultDigits),
			}), tokens.AccessToken))
	}); err != nil {
		t.Fatalf("confirm enrolment: %v", err)
	}

	// Password step, through a page we were actually served.
	client, csrf := signinPage(t)
	prompted := postLoginForm(t, client, url.Values{
		"tenant": {tenant}, "realm": {"internal"},
		"username": {username}, "password": {password},
		"csrf": {csrf},
	})
	body := readAll(t, prompted)
	prompted.Body.Close()
	if !strings.Contains(body, `name="mfa_token"`) {
		t.Fatalf("no second-factor form was rendered (status %d):\n%s",
			prompted.StatusCode, firstBytes(body, 400))
	}

	// Everything the form offers, and nothing else.
	form := formFields(body)
	if form.Get("username") != "" || form.Get("password") != "" {
		t.Fatal("the second-factor form carries credentials after all — " +
			"this test would be measuring the wrong thing")
	}
	if form.Get("mfa_token") == "" {
		t.Fatalf("the form carried no mfa_token: %v", form)
	}
	waitForNextStep()
	form.Set("code", totp.Generate(secret, time.Now(), totp.DefaultStep, totp.DefaultDigits))

	resp := postLoginForm(t, client, form)
	defer resp.Body.Close()
	out := readAll(t, resp)
	for _, c := range resp.Cookies() {
		if strings.Contains(c.Name, "anubis_sso") && c.Value != "" {
			return
		}
	}
	t.Fatalf("a correct code on the form the server rendered did not sign in "+
		"(status %d): the hosted second-factor step cannot be completed by a "+
		"browser\n%s", resp.StatusCode, firstBytes(out, 400))
}

var continueLink = regexp.MustCompile(`<a href="(/v1/authorize[^"]*)"`)

var setupKey = regexp.MustCompile(`<label for="k">Setup key</label>\s*<p class="sub"><code>([A-Z2-7]+)</code></p>`)

// A member the deadline refuses must be able to comply without leaving the
// page.
//
// This is the other half of TestBrowserLoginHonoursTheEnrolmentDeadline. That
// test proved the refusal is real; a refusal nobody can act on is a lockout.
// The API answers an overdue member with a grant token to enrol against, and
// the browser had nowhere to spend one — so the policy was unsatisfiable by
// exactly the population it applies to, which is everyone who only ever uses
// SSO.
//
// Every submission here is the form the server rendered, with the code typed
// into it. A hand-built post would prove nothing about what a browser can do.
func TestABrowserCanEnrolTheFactorItIsRefusedFor(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	token := platformLogin(t)
	admin := anubisv1connect.NewTenantAdminServiceClient(http.DefaultClient, baseURL)
	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)

	realmCode := fmt.Sprintf("bself%d", time.Now().UnixNano()%100_000_000)
	if _, err := admin.CreateRealm(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateRealmRequest{
		Realm: &anubisv1.Realm{
			Code: realmCode, Kind: "internal", DisplayName: "Browser self-enrolment",
			MinAssurance:    1,
			AllowedFactors:  []string{"password", "totp"},
			RequiredFactors: []string{"password", "totp"},
			SessionTtl:      "8 hours", AccessTokenTtl: "10 minutes",
			RefreshTokenTtl: "30 days",
			// Already overdue: this member is refused from the first attempt.
			FactorEnrolmentDeadline: time.Now().Add(-1 * time.Hour).Unix(),
		},
	}), token)); err != nil {
		t.Fatalf("create realm: %v", err)
	}

	username := fmt.Sprintf("bselfer-%d", time.Now().UnixNano())
	const password = "browser-self-enrol-password-1234"
	if _, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: realmCode, Username: username, Password: password,
	}), token)); err != nil {
		t.Fatalf("create identity: %v", err)
	}

	signedIn := func(resp *http.Response) string {
		t.Helper()
		for _, c := range resp.Cookies() {
			if strings.Contains(c.Name, "anubis_sso") && c.Value != "" {
				return c.Name
			}
		}
		return ""
	}
	passwordStep := func() (*http.Client, *http.Response, string) {
		t.Helper()
		client, form := signinPageForm(t)
		form.Set("realm", realmCode)
		form.Set("username", username)
		form.Set("password", password)
		resp := postLoginForm(t, client, form)
		return client, resp, readAll(t, resp)
	}

	// 1. Refused, and offered a way out rather than a dead end.
	client, refused, body := passwordStep()
	refused.Body.Close()
	if c := signedIn(refused); c != "" {
		t.Fatalf("an overdue member was signed in (cookie %q)", c)
	}
	if !strings.Contains(body, `name="enrol_token"`) {
		t.Fatalf("refused with no way to enrol (status %d):\n%s",
			refused.StatusCode, firstBytes(body, 500))
	}
	m := setupKey.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("the enrolment form carried no setup key:\n%s", firstBytes(body, 600))
	}
	secret := decodeBase32(t, m[1])

	// 2. Enrol, submitting exactly what the page offers.
	form := formFields(body)
	if form.Get("password") != "" {
		t.Fatal("the enrolment form asks for a password again")
	}
	waitForNextStep()
	form.Set("code", totp.Generate(secret, time.Now(), totp.DefaultStep, totp.DefaultDigits))
	enrolled := postLoginForm(t, client, form)
	enrolledBody := readAll(t, enrolled)
	enrolled.Body.Close()
	if c := signedIn(enrolled); c != "" {
		t.Fatalf("enrolling issued a session on its own (cookie %q) — enrolling "+
			"is not signing in, and the factor has not been presented yet", c)
	}
	// Recovery codes exist in readable form exactly once. If they are not on
	// this response they are gone.
	if !strings.Contains(enrolledBody, "recovery codes") {
		t.Fatalf("enrolment returned no recovery codes (status %d):\n%s",
			enrolled.StatusCode, firstBytes(enrolledBody, 600))
	}

	// 3. The member now holds the factor their realm demands, so signing in
	//    is CHALLENGED for it rather than refused for lacking it. That is the
	//    proof the enrolment actually took.
	client2, challenged, challengeBody := passwordStep()
	challenged.Body.Close()
	if strings.Contains(challengeBody, `name="enrol_token"`) {
		t.Fatalf("still asked to enrol after enrolling (status %d)", challenged.StatusCode)
	}
	if !strings.Contains(challengeBody, `name="mfa_token"`) {
		t.Fatalf("after enrolling, the page did not ask for the factor (status %d):\n%s",
			challenged.StatusCode, firstBytes(challengeBody, 500))
	}

	// 4. And the code completes it.
	second := formFields(challengeBody)
	waitForNextStep()
	second.Set("code", totp.Generate(secret, time.Now(), totp.DefaultStep, totp.DefaultDigits))
	done := postLoginForm(t, client2, second)
	doneBody := readAll(t, done)
	done.Body.Close()
	if signedIn(done) == "" {
		t.Fatalf("the factor enrolled through the browser did not complete a "+
			"sign-in (status %d):\n%s", done.StatusCode, firstBytes(doneBody, 500))
	}
}

// Inside the grace period the browser must sign somebody in AND tell them
// what is coming.
//
// The API returns the deadline and the missing factors on every sign-in
// inside the grace period. The browser returned nothing: that response
// redirects straight back to the application, so there was nowhere to put a
// warning, and a member who only ever used SSO met the policy for the first
// time on the day it refused them. The runway the deadline exists to provide
// did not reach them.
//
// Both properties are asserted here, because either alone is a regression:
// a warning that blocks is not a grace period, and a grace period that says
// nothing is not a warning.
func TestTheBrowserWarnsInsideTheGracePeriod(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	token := platformLogin(t)
	admin := anubisv1connect.NewTenantAdminServiceClient(http.DefaultClient, baseURL)
	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)

	realmCode := fmt.Sprintf("bgrace%d", time.Now().UnixNano()%100_000_000)
	deadline := time.Now().Add(72 * time.Hour)
	if _, err := admin.CreateRealm(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateRealmRequest{
		Realm: &anubisv1.Realm{
			Code: realmCode, Kind: "internal", DisplayName: "Browser grace period",
			MinAssurance:    1,
			AllowedFactors:  []string{"password", "totp"},
			RequiredFactors: []string{"password", "totp"},
			SessionTtl:      "8 hours", AccessTokenTtl: "10 minutes",
			RefreshTokenTtl:         "30 days",
			FactorEnrolmentDeadline: deadline.Unix(),
		},
	}), token)); err != nil {
		t.Fatalf("create realm: %v", err)
	}

	username := fmt.Sprintf("bgracee-%d", time.Now().UnixNano())
	const password = "browser-grace-password-1234"
	if _, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: realmCode, Username: username, Password: password,
	}), token)); err != nil {
		t.Fatalf("create identity: %v", err)
	}

	client, form := signinPageForm(t)
	form.Set("realm", realmCode)
	form.Set("username", username)
	form.Set("password", password)
	resp := postLoginForm(t, client, form)
	body := readAll(t, resp)
	resp.Body.Close()

	// 1. Signed in. The grace period must not refuse anybody.
	var signedIn bool
	for _, c := range resp.Cookies() {
		if strings.Contains(c.Name, "anubis_sso") && c.Value != "" {
			signedIn = true
		}
	}
	if !signedIn {
		t.Fatalf("the grace period refused a browser sign-in (status %d):\n%s",
			resp.StatusCode, firstBytes(body, 400))
	}

	// 2. And warned, naming the date and the factor.
	if !strings.Contains(body, deadline.Format("2 January 2006")) {
		t.Fatalf("signed in inside the grace period without naming the deadline "+
			"(status %d):\n%s", resp.StatusCode, firstBytes(body, 600))
	}
	if !strings.Contains(strings.ToLower(body), "authenticator") {
		t.Fatalf("warned without saying what to enrol:\n%s", firstBytes(body, 600))
	}

	// 3. The warning is skippable, and skipping it lands where the sign-in
	//    was going. A warning that strands the caller is worse than none.
	cont := continueLink.FindStringSubmatch(body)
	if cont == nil {
		t.Fatalf("the warning offered no way to continue:\n%s", firstBytes(body, 600))
	}
	onward, err := client.Get(baseURL + html.UnescapeString(cont[1]))
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	defer onward.Body.Close()
	if loc := onward.Header.Get("Location"); !strings.Contains(loc, "code=") {
		t.Fatalf("continuing from the warning did not issue a code (status %d, location %q)",
			onward.StatusCode, loc)
	}
}

// And the offer on that warning has to work: a member who complies early is
// enrolled and sent on, not asked for the password they just used.
func TestEnrolingFromTheWarningContinuesTheSignIn(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	token := platformLogin(t)
	admin := anubisv1connect.NewTenantAdminServiceClient(http.DefaultClient, baseURL)
	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)

	realmCode := fmt.Sprintf("bearly%d", time.Now().UnixNano()%100_000_000)
	if _, err := admin.CreateRealm(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateRealmRequest{
		Realm: &anubisv1.Realm{
			Code: realmCode, Kind: "internal", DisplayName: "Early complier",
			MinAssurance:    1,
			AllowedFactors:  []string{"password", "totp"},
			RequiredFactors: []string{"password", "totp"},
			SessionTtl:      "8 hours", AccessTokenTtl: "10 minutes",
			RefreshTokenTtl:         "30 days",
			FactorEnrolmentDeadline: time.Now().Add(72 * time.Hour).Unix(),
		},
	}), token)); err != nil {
		t.Fatalf("create realm: %v", err)
	}
	username := fmt.Sprintf("bearlye-%d", time.Now().UnixNano())
	const password = "browser-early-password-1234"
	if _, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: realmCode, Username: username, Password: password,
	}), token)); err != nil {
		t.Fatalf("create identity: %v", err)
	}

	client, form := signinPageForm(t)
	form.Set("realm", realmCode)
	form.Set("username", username)
	form.Set("password", password)
	warned := postLoginForm(t, client, form)
	warnBody := readAll(t, warned)
	warned.Body.Close()
	if !strings.Contains(warnBody, `name="enrol_now"`) {
		t.Fatalf("the warning did not offer to enrol (status %d):\n%s",
			warned.StatusCode, firstBytes(warnBody, 600))
	}

	// "Set it up now" — the form's own fields plus the button that was clicked.
	choice := formFields(warnBody)
	choice.Set("enrol_now", "1")
	offered := postLoginForm(t, client, choice)
	offerBody := readAll(t, offered)
	offered.Body.Close()
	m := setupKey.FindStringSubmatch(offerBody)
	if m == nil {
		t.Fatalf("no setup key offered to somebody complying early (status %d):\n%s",
			offered.StatusCode, firstBytes(offerBody, 600))
	}
	secret := decodeBase32(t, m[1])

	enrolForm := formFields(offerBody)
	waitForNextStep()
	enrolForm.Set("code", totp.Generate(secret, time.Now(), totp.DefaultStep, totp.DefaultDigits))
	done := postLoginForm(t, client, enrolForm)
	doneBody := readAll(t, done)
	done.Body.Close()

	if !strings.Contains(doneBody, "recovery codes") {
		t.Fatalf("enrolling early returned no recovery codes (status %d):\n%s",
			done.StatusCode, firstBytes(doneBody, 600))
	}
	// Already signed in before enrolling, so the password must not be asked
	// for again — complying early cannot cost more than skipping.
	if strings.Contains(doneBody, `name="password"`) {
		t.Fatalf("after enrolling early the page asked for the password again:\n%s",
			firstBytes(doneBody, 600))
	}
	cont := continueLink.FindStringSubmatch(doneBody)
	if cont == nil {
		t.Fatalf("no way onward after enrolling early:\n%s", firstBytes(doneBody, 600))
	}
	onward, err := client.Get(baseURL + html.UnescapeString(cont[1]))
	if err != nil {
		t.Fatalf("continue: %v", err)
	}
	defer onward.Body.Close()
	if loc := onward.Header.Get("Location"); !strings.Contains(loc, "code=") {
		t.Fatalf("continuing after enrolling did not issue a code (status %d, location %q)",
			onward.StatusCode, loc)
	}
}
