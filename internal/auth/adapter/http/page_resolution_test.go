package authhttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"testing"

	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// Page resolution decides which door a person sees, and the order is the whole
// feature: slug -> application -> realm -> tenant default. Getting it wrong is
// not a crash, it is the wrong brand in front of the wrong population — and
// for a partner portal that means showing suppliers an employee page.
//
// The realm step was inserted between application and default so that nothing
// which already resolved changes. These pin that.

var errNoPage = errors.New("no such page")

type stubPages struct {
	tenancyport.AuthPageRepository
	bySlug, byApp, byRealm, byDefault string // page name, empty = not found
	asked                             []string
}

func page(name string) *tenancydomain.AuthPage {
	return &tenancydomain.AuthPage{Name: name, Config: []byte(`{"brand":{"title":"` + name + `"}}`)}
}

func (s *stubPages) AuthPageBySlug(_ context.Context, _, _, _ string) (*tenancydomain.AuthPage, error) {
	s.asked = append(s.asked, "slug")
	if s.bySlug == "" {
		return nil, errNoPage
	}
	return page(s.bySlug), nil
}

func (s *stubPages) AuthPageForApplication(_ context.Context, _, _, _ string) (*tenancydomain.AuthPage, error) {
	s.asked = append(s.asked, "application")
	if s.byApp == "" {
		return nil, errNoPage
	}
	return page(s.byApp), nil
}

func (s *stubPages) AuthPageForRealm(_ context.Context, _, _, _ string) (*tenancydomain.AuthPage, error) {
	s.asked = append(s.asked, "realm")
	if s.byRealm == "" {
		return nil, errNoPage
	}
	return page(s.byRealm), nil
}

func (s *stubPages) DefaultAuthPage(_ context.Context, _, _ string) (*tenancydomain.AuthPage, error) {
	s.asked = append(s.asked, "default")
	if s.byDefault == "" {
		return nil, errNoPage
	}
	return page(s.byDefault), nil
}

type stubRealms struct {
	identityport.RealmRepository
	id string
}

func (s *stubRealms) RealmByCode(_ context.Context, _, code string) (*identitydomain.Realm, error) {
	if s.id == "" {
		return nil, errNoPage
	}
	return &identitydomain.Realm{ID: s.id, Code: code}, nil
}

func resolve(t *testing.T, p *stubPages, realmID, realmCode string) string {
	t.Helper()
	h := &OIDCHandler{pages: p, realms: &stubRealms{id: realmID}, logger: slog.Default()}
	r := httptest.NewRequest("GET", "/p/acme/signin", nil)
	cfg := h.resolvePageForRealm(r, "tenant-1", "signin", "the-slug", "app-1", realmCode)
	return cfg.Brand.Title
}

func TestSlugWinsOverEverything(t *testing.T) {
	p := &stubPages{bySlug: "by-slug", byApp: "by-app", byRealm: "by-realm", byDefault: "by-default"}
	if got := resolve(t, p, "realm-1", "partner"); got != "by-slug" {
		t.Errorf("resolved %q, want the explicitly requested page", got)
	}
}

// An application that configured its own door keeps it. This is the property
// that makes adding realms safe: no installation that resolves today changes.
func TestApplicationBeatsRealm(t *testing.T) {
	p := &stubPages{byApp: "by-app", byRealm: "by-realm", byDefault: "by-default"}
	if got := resolve(t, p, "realm-1", "partner"); got != "by-app" {
		t.Errorf("resolved %q — adding a realm page changed an application that already resolved", got)
	}
}

// The point of the feature: partners see the partner door rather than the
// tenant default.
func TestRealmBeatsTheDefault(t *testing.T) {
	p := &stubPages{byRealm: "by-realm", byDefault: "by-default"}
	if got := resolve(t, p, "realm-1", "partner"); got != "by-realm" {
		t.Errorf("resolved %q, want the realm's own page", got)
	}
}

func TestFallsThroughToTheDefault(t *testing.T) {
	p := &stubPages{byDefault: "by-default"}
	if got := resolve(t, p, "realm-1", "partner"); got != "by-default" {
		t.Errorf("resolved %q, want the tenant default", got)
	}
}

// No realm in the request — sign-out, or a caller that never resolved one.
// The realm lookup must not run at all, rather than running with an empty
// code and matching whatever sorts first.
func TestNoRealmCodeSkipsTheRealmLookup(t *testing.T) {
	p := &stubPages{byRealm: "by-realm", byDefault: "by-default"}
	if got := resolve(t, p, "realm-1", ""); got != "by-default" {
		t.Errorf("resolved %q with no realm supplied, want the default", got)
	}
	for _, step := range p.asked {
		if step == "realm" {
			t.Error("queried for a realm page without a realm code")
		}
	}
}

// An unknown realm code must fall through, not fail the render. A stale
// ?realm= in a bookmark should show the default door, not an error page.
func TestAnUnknownRealmFallsThrough(t *testing.T) {
	p := &stubPages{byRealm: "by-realm", byDefault: "by-default"}
	if got := resolve(t, p, "", "does-not-exist"); got != "by-default" {
		t.Errorf("resolved %q for an unknown realm, want the default", got)
	}
}

// Nothing configured at all still renders: pagecfg supplies defaults rather
// than the page failing to load.
func TestNothingConfiguredStillRenders(t *testing.T) {
	p := &stubPages{}
	if got := resolve(t, p, "realm-1", "partner"); got == "" {
		t.Error("an unconfigured tenant rendered no brand title at all")
	}
}
