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
	token := login(t).AccessToken

	// Register an application that permits exactly one return address.
	slug := fmt.Sprintf("logout-probe-%d", time.Now().UnixNano()%1_000_000)
	const allowed = "https://allowed.example/after-logout"
	if _, err := pageClient().CreateApplication(ctx, bearer(connect.NewRequest(&anubisv1.CreateApplicationRequest{
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
