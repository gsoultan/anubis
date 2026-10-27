package authhttprecovery

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	identityrecovery "github.com/gsoultan/anubis/internal/identity/app/recovery"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/ratelimit"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

type stubTenants struct{ tenancyport.TenantRepository }

func (stubTenants) TenantBySlug(_ context.Context, slug string) (*tenancydomain.TenantRef, error) {
	if slug != "acme" {
		return nil, errors.New("no such tenant")
	}
	return &tenancydomain.TenantRef{ID: "tenant-1", Slug: "acme"}, nil
}

type stubRealms struct{ identityport.RealmRepository }

func (stubRealms) RealmByCode(context.Context, string, string) (*identitydomain.Realm, error) {
	return &identitydomain.Realm{ID: "realm-1", Code: "internal",
		PasswordPolicy: identitydomain.PasswordPolicy{MinLength: 14}}, nil
}

type stubPages struct{ tenancyport.AuthPageRepository }

func (stubPages) AuthPageForRealm(context.Context, string, string, string) (*tenancydomain.AuthPage, error) {
	return nil, errors.New("none")
}

func (stubPages) DefaultAuthPage(context.Context, string, string) (*tenancydomain.AuthPage, error) {
	return &tenancydomain.AuthPage{Config: []byte(`{"brand":{"title":"Acme Corp"}}`)}, nil
}

// fakeRecovery records what the pages asked for and answers as told.
type fakeRecovery struct {
	requests    []identityrecovery.RequestResetInput
	completed   [][2]string
	requestErr  error
	completeErr error
}

func (f *fakeRecovery) RequestPasswordReset(_ context.Context, in identityrecovery.RequestResetInput) error {
	f.requests = append(f.requests, in)
	return f.requestErr
}

func (f *fakeRecovery) CompletePasswordReset(_ context.Context, token, password string) error {
	f.completed = append(f.completed, [2]string{token, password})
	return f.completeErr
}

func (f *fakeRecovery) ChangePassword(context.Context, string, string) error { return nil }

func newMux(t *testing.T, svc *fakeRecovery) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	NewHandler(svc, stubTenants{}, stubRealms{}, stubPages{}, ratelimit.New(),
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))).Mount(
		func(pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, h) })
	return mux
}

func get(mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func post(mux *http.ServeMux, path string, form url.Values) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(rec, req)
	return rec
}

func TestTheRequestPageAsksForAUsernameOnTheTenantsBrand(t *testing.T) {
	rec := get(newMux(t, &fakeRecovery{}), "/p/acme/reset/request?realm=internal")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Acme Corp", "Reset your password", `action="/p/acme/reset/request"`,
		`name="username"`, `name="realm" value="internal"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
}

// The answer is "check your email" whatever the usecase found, and the
// username reaches it with the tenant and realm from the page, not from
// anything the browser could rewrite.
func TestSubmittingAUsernameAlwaysSaysCheckYourEmail(t *testing.T) {
	svc := &fakeRecovery{}
	mux := newMux(t, svc)
	rec := post(mux, "/p/acme/reset/request", url.Values{"realm": {"internal"}, "username": {"dewi"}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Check your email") {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
	if len(svc.requests) != 1 || svc.requests[0] != (identityrecovery.RequestResetInput{
		Tenant: "acme", Realm: "internal", Username: "dewi"}) {
		t.Fatalf("usecase asked with %+v", svc.requests)
	}
}

// Three requests a minute to one account; the fourth is refused with a
// Retry-After, before the usecase is asked.
func TestAFourthRequestForOneAccountIsRateLimited(t *testing.T) {
	svc := &fakeRecovery{}
	mux := newMux(t, svc)
	form := url.Values{"realm": {"internal"}, "username": {"dewi"}}
	for i := 0; i < 3; i++ {
		if rec := post(mux, "/p/acme/reset/request", form); rec.Code != http.StatusOK {
			t.Fatalf("request %d: status %d", i+1, rec.Code)
		}
	}
	rec := post(mux, "/p/acme/reset/request", form)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("fourth request: status %d retry-after %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if len(svc.requests) != 3 {
		t.Fatalf("the limited request still reached the usecase: %d calls", len(svc.requests))
	}
}

// A server that cannot send mail says so to everybody — the one error the
// usecase returns is not about the account.
func TestNoMailIsSaidPlainly(t *testing.T) {
	rec := post(newMux(t, &fakeRecovery{requestErr: apperr.ErrMailNotConfigured}),
		"/p/acme/reset/request", url.Values{"realm": {"internal"}, "username": {"dewi"}})
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "cannot send email") {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
}

// The link opens a form that carries the token and the realm's minimum
// length — and consumes nothing yet, so a mail client prefetching the URL
// does not burn the reset.
func TestTheLinkOpensAFormWithoutConsumingTheToken(t *testing.T) {
	svc := &fakeRecovery{}
	rec := get(newMux(t, svc), "/p/acme/reset/complete?realm=internal&token=abc123")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`name="token" value="abc123"`, `minlength="14"`, "At least 14 characters",
		`autocomplete="new-password"`, `action="/p/acme/reset/complete"`} {
		if !strings.Contains(body, want) {
			t.Errorf("form lacks %q", want)
		}
	}
	if len(svc.completed) != 0 {
		t.Fatal("opening the link consumed the token")
	}
	if rec := get(newMux(t, svc), "/p/acme/reset/complete"); rec.Code != http.StatusBadRequest {
		t.Fatalf("a link without a token rendered %d", rec.Code)
	}
}

func TestSavingThePasswordReportsTheOutcomeHonestly(t *testing.T) {
	form := url.Values{"realm": {"internal"}, "token": {"abc123"}, "password": {"a-long-enough-password"}}

	svc := &fakeRecovery{}
	rec := post(newMux(t, svc), "/p/acme/reset/complete", form)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Your password is set") {
		t.Fatalf("success: status %d body %q", rec.Code, rec.Body.String())
	}
	if len(svc.completed) != 1 || svc.completed[0] != [2]string{"abc123", "a-long-enough-password"} {
		t.Fatalf("usecase asked with %v", svc.completed)
	}
	if !strings.Contains(rec.Body.String(), `/p/acme/signin/default?realm=internal`) {
		t.Fatal("the done page does not lead to sign-in")
	}

	rec = post(newMux(t, &fakeRecovery{completeErr: apperr.ErrTokenInvalid}), "/p/acme/reset/complete", form)
	body := rec.Body.String()
	if rec.Code != http.StatusBadRequest || !strings.Contains(body, "expired") ||
		!strings.Contains(body, "/p/acme/reset/request?realm=internal") {
		t.Fatalf("expired: status %d body %q", rec.Code, body)
	}
	if strings.Contains(body, `name="password"`) {
		t.Fatal("an expired link still offers a password field")
	}

	rec = post(newMux(t, &fakeRecovery{completeErr: apperr.ErrPasswordPolicy.With("min_length", "14")}),
		"/p/acme/reset/complete", form)
	body = rec.Body.String()
	if rec.Code != http.StatusBadRequest || !strings.Contains(body, "at least 14 characters") ||
		!strings.Contains(body, "request a new one") {
		t.Fatalf("policy: status %d body %q", rec.Code, body)
	}
}

// A tenant that does not exist still gets a page: a 404 on the reset URL
// would say which slugs are real.
func TestAnUnknownTenantStillGetsAPage(t *testing.T) {
	rec := get(newMux(t, &fakeRecovery{}), "/p/nope/reset/request")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Reset your password") {
		t.Fatalf("status %d", rec.Code)
	}
}
