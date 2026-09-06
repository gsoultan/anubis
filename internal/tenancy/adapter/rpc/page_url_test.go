package tenancyrpc

import (
	"context"
	"testing"

	"github.com/gsoultan/anubis/internal/shared/authctx"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

// The console shows this URL with a copy button, so it is a published address
// in every sense that matters: somebody pastes it into a link and expects it
// to open the page they were just editing.
//
// It used to be built from cfg.DefaultTenant for every caller. A platform
// operator administering any OTHER tenant — which is the normal case, since
// the console has a tenant picker — was handed a URL naming the default
// tenant. ServePage resolves the tenant from the path, so that link 404s
// while the page it names exists and works. Wrong in the worst direction: it
// looks correct.
func TestThePageURLNamesTheTenantBeingAdministered(t *testing.T) {
	h := &TenantAdminHandler{issuer: "https://id.example.com", tenantSlug: "bootstrap"}
	page := tenancydomain.AuthPage{Kind: "signin", Slug: "default"}

	ctx := authctx.With(context.Background(), &authctx.Principal{
		Platform: true, TenantSlug: "acme",
	})

	got := h.pageURL(ctx, page)
	want := "https://id.example.com/p/acme/signin/default"
	if got != want {
		t.Errorf("URL names the wrong tenant\n got: %s\nwant: %s", got, want)
	}
}

// A caller with no tenant on the principal still gets something useful rather
// than a blank field, which is why the configured default survives.
func TestThePageURLFallsBackToTheConfiguredTenant(t *testing.T) {
	h := &TenantAdminHandler{issuer: "https://id.example.com", tenantSlug: "bootstrap"}
	page := tenancydomain.AuthPage{Kind: "signout", Slug: "bye"}

	for name, ctx := range map[string]context.Context{
		"no principal": context.Background(),
		"no tenant on the principal": authctx.With(
			context.Background(), &authctx.Principal{Platform: true}),
	} {
		if got, want := h.pageURL(ctx, page), "https://id.example.com/p/bootstrap/signout/bye"; got != want {
			t.Errorf("%s: got %s, want %s", name, got, want)
		}
	}
}

// Empty rather than a half-built URL: the console renders the shape as a hint
// instead, and a string like "/p//signin/default" would be worse than nothing
// because it is copyable.
func TestThePageURLIsEmptyWithNothingToBuildFrom(t *testing.T) {
	page := tenancydomain.AuthPage{Kind: "signin", Slug: "default"}
	for name, h := range map[string]*TenantAdminHandler{
		"no issuer": {issuer: "", tenantSlug: "acme"},
		"no tenant": {issuer: "https://id.example.com", tenantSlug: ""},
	} {
		if got := h.pageURL(context.Background(), page); got != "" {
			t.Errorf("%s: expected an empty URL, got %q", name, got)
		}
	}
}

// pageURL being right is only half of it: the URL has to survive onto the
// message the console actually reads. The field existed on the wire for a
// while with nothing filling it in the console, which is the same class of
// gap in the other direction — so this pins the whole path, ctx to proto.
func TestThePageURLReachesTheWireMessage(t *testing.T) {
	h := &TenantAdminHandler{issuer: "https://id.example.com", tenantSlug: "bootstrap"}
	ctx := authctx.With(context.Background(), &authctx.Principal{
		Platform: true, TenantSlug: "acme",
	})

	msg := h.pageProto(ctx, tenancydomain.AuthPage{
		Kind: "signin", Slug: "partners", Name: "Partner sign-in",
	})

	if msg.Url != "https://id.example.com/p/acme/signin/partners" {
		t.Errorf("Url on the wire message is %q", msg.Url)
	}
}
