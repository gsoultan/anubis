//go:build integration

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
)

var csrfField = regexp.MustCompile(`name="csrf" value="([^"]*)"`)

// RP-initiated logout. Two properties carry the security weight: the
// confirmation cannot be forged from another site, and the return address
// cannot be anything the caller feels like.
func TestRPInitiatedLogout(t *testing.T) {
	requireServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// GET asks first. A bare GET that ended sessions would let any page on
	// the internet sign users out with an <img> tag.
	resp, err := client.Get(baseURL + "/v1/logout?tenant=" + tenant)
	if err != nil {
		t.Fatal(err)
	}
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout page: status %d", resp.StatusCode)
	}
	m := csrfField.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("no CSRF token in the sign-out form: the confirmation proves nothing")
	}
	if strings.Contains(body, "You have been signed out") {
		t.Fatal("GET /v1/logout reported a completed sign-out without confirmation")
	}

	// A POST without the token — the shape a cross-site form submission
	// takes — must not end the session.
	forged, err := client.PostForm(baseURL+"/v1/logout", url.Values{
		"tenant": {tenant}, "csrf": {"forged-value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fb := readAll(t, forged); strings.Contains(fb, "You have been signed out") {
		t.Fatal("a forged CSRF token completed a sign-out")
	}

	// The rejected attempt re-rendered the form, which ROTATES the token —
	// a confirmation token that survived a failed submission would be
	// replayable. So fetch the page again, exactly as a user would see it.
	resp2, err := client.Get(baseURL + "/v1/logout?tenant=" + tenant)
	if err != nil {
		t.Fatal(err)
	}
	m2 := csrfField.FindStringSubmatch(readAll(t, resp2))
	if m2 == nil {
		t.Fatal("no CSRF token on the re-rendered form")
	}
	if m2[1] == m[1] {
		t.Fatal("the sign-out token did not rotate after a rejected attempt")
	}

	// The genuine confirmation works.
	done, err := client.PostForm(baseURL+"/v1/logout", url.Values{
		"tenant": {tenant}, "csrf": {m2[1]},
	})
	if err != nil {
		t.Fatal(err)
	}
	if db := readAll(t, done); !strings.Contains(db, "You have been signed out") {
		t.Fatalf("confirmed sign-out did not complete: %.200s", db)
	}
}

// post_logout_redirect_uri is an open-redirect vector: "you have been signed
// out, sign in again here" is far more convincing when the link genuinely
// came from the identity provider. Only registered addresses are honoured.
func TestLogoutRedirectMustBeRegistered(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	token := platformLogin(t)

	// Register an application that permits exactly one return address.
	slug := uniqueSlug("logout-probe")
	const allowed = "https://allowed.example/after-logout"
	if _, err := pageClient().CreateApplication(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateApplicationRequest{
		Application: &anubisv1.Application{
			Slug: slug, Name: "Logout probe", Kind: "spa",
			RedirectUris:           []string{"https://allowed.example/callback"},
			PostLogoutRedirectUris: []string{allowed},
		},
	}), token)); err != nil {
		t.Fatalf("create application: %v", err)
	}

	get := func(target string) string {
		u := baseURL + "/v1/logout?tenant=" + tenant +
			"&post_logout_redirect_uri=" + url.QueryEscape(target)
		resp, err := http.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		return readAll(t, resp)
	}

	// An unregistered address is refused — and the user is told, rather than
	// silently sent somewhere else.
	body := get("https://evil.example/phish")
	if strings.Contains(body, "evil.example") {
		t.Fatal("an unregistered return address reached the page")
	}
	if !strings.Contains(body, "not registered") {
		t.Fatalf("no explanation for the rejected return address: %.200s", body)
	}

	// The registered one is offered.
	if ok := get(allowed); !strings.Contains(ok, "allowed.example") {
		t.Fatalf("registered return address was not honoured: %.200s", ok)
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b := make([]byte, 0, 4096)
	buf := make([]byte, 2048)
	for {
		n, err := resp.Body.Read(buf)
		b = append(b, buf[:n]...)
		if err != nil {
			break
		}
	}
	return string(b)
}

// A cross-site form must not be able to sign somebody in.
//
// The logout form carries a CSRF token, and checkLogoutCSRF says why:
// "without it the confirmation is decorative: any site could submit the form
// for you". POST /v1/login carried nothing, and there is no Origin or
// Referer check either — so any page could auto-submit a login form with the
// ATTACKER's credentials, and the victim's browser would store an
// __Host-anubis_sso cookie for the attacker's account.
//
// SameSite=Lax does not stop it: SameSite governs whether a cookie is SENT,
// not whether one can be SET, and Lax then sends it on the victim's next
// top-level navigation to the issuer. The victim proceeds through the OIDC
// flow and the relying party receives the attacker's identity — so whatever
// the human does next belongs to somebody else's account.
func TestLoginRefusesACrossSiteSubmission(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	adminToken := platformLogin(t)

	username := fmt.Sprintf("csrf-probe-%d", time.Now().UnixNano())
	const password = "csrf-probe-password-1234"

	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)
	if _, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: "internal", Username: username, Password: password, AssuranceLevel: 1,
	}), adminToken)); err != nil {
		t.Fatalf("create probe identity: %v", err)
	}

	// No prior GET of the sign-in page: a cross-site submission has never
	// been served one, so it holds none of the state the page hands out.
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp := postLoginForm(t, client, url.Values{
		"tenant": {tenant}, "realm": {"internal"},
		"username": {username}, "password": {password},
	})
	defer resp.Body.Close()

	for _, c := range resp.Cookies() {
		if strings.Contains(c.Name, "anubis_sso") && c.Value != "" {
			t.Fatalf("a submission with no page state signed the browser in "+
				"(cookie %q, status %d): any site could do this with its own "+
				"credentials", c.Name, resp.StatusCode)
		}
	}
}

// signinPage registers an application, renders the hosted sign-in page
// through /v1/authorize, and returns a client holding that page's state plus
// the CSRF token the form carries.
//
// The GET matters: a sign-in form is only submittable by somebody who was
// served one, which is the property TestLoginRefusesACrossSiteSubmission
// exists to keep.
func signinPageBody(t *testing.T) (*http.Client, string) {
	t.Helper()
	ctx := context.Background()
	token := platformLogin(t)

	slug := uniqueSlug("csrf-app")
	if _, err := pageClient().CreateApplication(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateApplicationRequest{
		Application: &anubisv1.Application{
			Slug: slug, Name: "CSRF probe", Kind: "spa",
			RedirectUris: []string{"https://allowed.example/callback"},
		},
	}), token)); err != nil {
		t.Fatalf("create application: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	q := url.Values{
		"tenant": {tenant}, "client_id": {slug},
		"redirect_uri":          {"https://allowed.example/callback"},
		"response_type":         {"code"},
		"code_challenge":        {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"},
		"code_challenge_method": {"S256"},
	}
	resp, err := client.Get(baseURL + "/v1/authorize?" + q.Encode())
	if err != nil {
		t.Fatalf("render sign-in page: %v", err)
	}
	return client, readAll(t, resp)
}

func signinPage(t *testing.T) (*http.Client, string) {
	t.Helper()
	client, body := signinPageBody(t)
	m := csrfField.FindStringSubmatch(body)
	if m == nil || m[1] == "" {
		t.Fatalf("no CSRF token in the sign-in form (status %d): a cross-site "+
			"form would be indistinguishable from a real one", len(body))
	}
	return client, m[1]
}

// signinPageForm returns the rendered form itself, so a test submits what the
// page carries instead of the three fields it happens to remember. Anything
// built from those hidden fields later — the continue URL after a
// grace-period warning, for one — is empty otherwise, and the failure looks
// like a server bug.
func signinPageForm(t *testing.T) (*http.Client, url.Values) {
	t.Helper()
	client, body := signinPageBody(t)
	form := formFields(body)
	if form.Get("csrf") == "" {
		t.Fatalf("the sign-in form carried no CSRF token:\n%s", body[:min(len(body), 400)])
	}
	if form.Get("client_id") == "" {
		t.Fatal("the sign-in form carried no client_id — a flow resumed from " +
			"it would have nowhere to go")
	}
	return client, form
}

// The page's own form must still work, or the check closes the hole by
// removing sign-in.
func TestLoginFromTheRenderedPageWorks(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	adminToken := platformLogin(t)

	username := fmt.Sprintf("csrf-ok-%d", time.Now().UnixNano())
	const password = "csrf-ok-password-1234"
	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)
	if _, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: "internal", Username: username, Password: password, AssuranceLevel: 1,
	}), adminToken)); err != nil {
		t.Fatalf("create probe identity: %v", err)
	}

	client, csrf := signinPage(t)
	resp := postLoginForm(t, client, url.Values{
		"tenant": {tenant}, "realm": {"internal"},
		"username": {username}, "password": {password},
		"csrf": {csrf},
	})
	defer resp.Body.Close()

	var signedIn bool
	for _, c := range resp.Cookies() {
		if strings.Contains(c.Name, "anubis_sso") && c.Value != "" {
			signedIn = true
		}
	}
	if !signedIn {
		t.Fatalf("a submission from the rendered page did not sign in (status %d)",
			resp.StatusCode)
	}
}

// formFields is what a browser would submit from a rendered form: every
// input the server actually put on the page, and nothing that is not there.
//
// Hand-built form values are the trap this avoids. A test that posts a field
// the page does not contain is testing a client nobody ships — it can pass
// against a form no human could submit.
var (
	inputTag = regexp.MustCompile(`<input\b[^>]*>`)
	tagAttr  = regexp.MustCompile(`([a-zA-Z][\w-]*)="([^"]*)"`)
)

func formFields(html string) url.Values {
	out := url.Values{}
	for _, tag := range inputTag.FindAllString(html, -1) {
		attrs := map[string]string{}
		for _, m := range tagAttr.FindAllStringSubmatch(tag, -1) {
			attrs[m[1]] = m[2]
		}
		switch {
		case attrs["name"] == "":
		case attrs["type"] == "submit", attrs["type"] == "checkbox":
		default:
			out.Set(attrs["name"], attrs["value"])
		}
	}
	return out
}
