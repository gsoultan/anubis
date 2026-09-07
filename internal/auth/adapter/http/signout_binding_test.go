package authhttp

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// Signing in and signing out used to disagree about who you are.
//
// Sign-in resolves slug -> application -> population -> default. Sign-out
// passed neither an application nor a population, so it collapsed to
// "explicit page= or the tenant default" — yet the console offers both
// bindings for sign-out pages and the schema indexes them
// (auth_pages_one_per_app, auth_pages_one_per_realm). A partner got the
// partner door on the way in and a generic one on the way out, and nothing
// anywhere said why.
//
// Everything needed was already at hand: performLogout reads the SSO cookie
// to revoke the session, and SessionView carries ApplicationID and RealmCode.

type stubTenants struct {
	tenancyport.TenantRepository
	id, slug string
}

func (s *stubTenants) TenantBySlug(_ context.Context, slug string) (*tenancydomain.TenantRef, error) {
	if slug != s.slug {
		return nil, errNoPage
	}
	return &tenancydomain.TenantRef{ID: s.id, Slug: s.slug}, nil
}

type stubSessions struct {
	authport.SessionRepository
	view *authdomain.SessionView
}

func (s *stubSessions) SessionByCookieHash(_ context.Context, _ []byte) (*authdomain.SessionView, error) {
	if s.view == nil {
		return nil, errNoPage
	}
	return s.view, nil
}

// signoutPage carries confirm:true so the GET stops at the confirmation and
// the test does not have to stand up revocation, refresh and audit to observe
// which page was chosen.
func signoutPageCfg(name string) *tenancydomain.AuthPage {
	return &tenancydomain.AuthPage{
		Name:   name,
		Config: []byte(`{"brand":{"title":"` + name + `"},"behavior":{"confirm":true}}`),
	}
}

type signoutPages struct{ stubPages }

func (s *signoutPages) AuthPageBySlug(ctx context.Context, a, b, c string) (*tenancydomain.AuthPage, error) {
	if _, err := s.stubPages.AuthPageBySlug(ctx, a, b, c); err != nil {
		return nil, err
	}
	return signoutPageCfg(s.bySlug), nil
}

func (s *signoutPages) AuthPageForApplication(ctx context.Context, a, b, c string) (*tenancydomain.AuthPage, error) {
	if _, err := s.stubPages.AuthPageForApplication(ctx, a, b, c); err != nil {
		return nil, err
	}
	return signoutPageCfg(s.byApp), nil
}

func (s *signoutPages) AuthPageForRealm(ctx context.Context, a, b, c string) (*tenancydomain.AuthPage, error) {
	if _, err := s.stubPages.AuthPageForRealm(ctx, a, b, c); err != nil {
		return nil, err
	}
	return signoutPageCfg(s.byRealm), nil
}

func (s *signoutPages) DefaultAuthPage(ctx context.Context, a, b string) (*tenancydomain.AuthPage, error) {
	if _, err := s.stubPages.DefaultAuthPage(ctx, a, b); err != nil {
		return nil, err
	}
	return signoutPageCfg(s.byDefault), nil
}

func logoutGET(t *testing.T, p *signoutPages, realms *stubRealms,
	view *authdomain.SessionView, withCookie bool) string {
	t.Helper()

	h := &OIDCHandler{
		tenants:  &stubTenants{id: "tenant-1", slug: "acme"},
		sessions: &stubSessions{view: view},
		pages:    p,
		realms:   realms,
		renderer: NewPageRenderer(),
		logger:   slog.Default(),
	}

	r := httptest.NewRequest("GET", "/v1/logout?tenant=acme", nil)
	if withCookie {
		r.AddCookie(&http.Cookie{Name: ssoCookieBase, Value: "an-sso-cookie"})
	}
	w := httptest.NewRecorder()
	h.LogoutPage(w, r)
	return w.Body.String()
}

func TestSignOutPicksThePopulationsPage(t *testing.T) {
	p := &signoutPages{stubPages{byRealm: "partner-signout", byDefault: "generic-signout"}}
	realms := &stubRealms{id: "realm-1"}

	body := logoutGET(t, p, realms, &authdomain.SessionView{
		TenantID: "tenant-1", RealmCode: "partner",
	}, true)

	if !strings.Contains(body, "partner-signout") {
		t.Errorf("a partner signing out got the generic page; body did not mention the partner page")
	}
	if realms.codeAsked != "partner" {
		t.Errorf("realm lookup asked for %q, want the session's realm code", realms.codeAsked)
	}
}

// The application binding outranks the population, exactly as at sign-in.
func TestSignOutPrefersTheApplicationOverThePopulation(t *testing.T) {
	p := &signoutPages{stubPages{byApp: "app-signout", byRealm: "partner-signout", byDefault: "generic"}}

	body := logoutGET(t, p, &stubRealms{id: "realm-1"}, &authdomain.SessionView{
		TenantID: "tenant-1", ApplicationID: "app-7", RealmCode: "partner",
	}, true)

	if !strings.Contains(body, "app-signout") {
		t.Error("the application's own sign-out page did not win")
	}
	if p.appAsked != "app-7" {
		t.Errorf("application lookup asked for %q, want the session's application", p.appAsked)
	}
}

// Nobody signed in, nothing to learn from — the default, as before.
func TestSignOutWithoutASessionFallsBackToTheDefault(t *testing.T) {
	p := &signoutPages{stubPages{byRealm: "partner-signout", byDefault: "generic-signout"}}

	body := logoutGET(t, p, &stubRealms{id: "realm-1"}, nil, false)

	if !strings.Contains(body, "generic-signout") {
		t.Error("no cookie should resolve to the tenant default")
	}
}

// Another tenant's cookie must not choose this tenant's page. The session is
// still returned for revocation — that is deliberate, and covered below — but
// it may not influence which door is drawn.
func TestAForeignSessionDoesNotChooseThePage(t *testing.T) {
	p := &signoutPages{stubPages{byRealm: "partner-signout", byDefault: "generic-signout"}}

	body := logoutGET(t, p, &stubRealms{id: "realm-1"}, &authdomain.SessionView{
		TenantID: "some-other-tenant", RealmCode: "partner",
	}, true)

	if !strings.Contains(body, "generic-signout") {
		t.Error("a session belonging to another tenant selected this tenant's page")
	}
}

// signoutBindings is where the tenant check lives, and it is the piece a
// future caller is most likely to reuse without one.
func TestSignoutBindingsIsScopedToTheTenant(t *testing.T) {
	for name, tc := range map[string]struct {
		view           *authdomain.SessionView
		wantApp, wantR string
	}{
		"nil session": {nil, "", ""},
		"same tenant": {
			&authdomain.SessionView{TenantID: "t1", ApplicationID: "a1", RealmCode: "partner"},
			"a1", "partner",
		},
		"other tenant": {
			&authdomain.SessionView{TenantID: "t2", ApplicationID: "a1", RealmCode: "partner"},
			"", "",
		},
	} {
		app, realm := signoutBindings(tc.view, "t1")
		if app != tc.wantApp || realm != tc.wantR {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", name, app, realm, tc.wantApp, tc.wantR)
		}
	}
}
