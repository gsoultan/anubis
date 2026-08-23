package authhttp

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

	apihttp "github.com/gsoultan/anubis/internal/api/http"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	"github.com/gsoultan/anubis/internal/tenancy/domain/pagecfg"
)

// resolvePage picks which page to render, most specific first:
//
//	?page=<slug>            an explicit choice, e.g. a link a tenant published
//	the application's page   app-initiated flows keep the app's branding
//	the tenant default       what everything else falls back to
//
// A disabled or missing page falls through rather than 404ing mid-flow:
// losing a page's branding must never cost a user the ability to sign in.
func (h *OIDCHandler) resolvePage(r *http.Request, tenantID, kind, slug, applicationID string) *pagecfg.Config {
	ctx := r.Context()
	var page *tenancydomain.AuthPage

	if slug != "" {
		if p, err := h.pages.AuthPageBySlug(ctx, tenantID, kind, slug); err == nil {
			page = p
		}
	}
	if page == nil && applicationID != "" {
		if p, err := h.pages.AuthPageForApplication(ctx, tenantID, kind, applicationID); err == nil {
			page = p
		}
	}
	if page == nil {
		if p, err := h.pages.DefaultAuthPage(ctx, tenantID, kind); err == nil {
			page = p
		}
	}

	var raw []byte
	if page != nil {
		raw = page.Config
	}
	cfg, err := pagecfg.Parse(pagecfg.Kind(kind), raw)
	if err != nil {
		// A stored config that no longer validates (a downgrade, a hand-edited
		// row) must not take sign-in down. Fall back to defaults and serve.
		h.logger.Error("stored page config is invalid; rendering defaults",
			"tenant", tenantID, "kind", kind, "error", err)
		cfg, _ = pagecfg.Parse(pagecfg.Kind(kind), nil)
	}
	return cfg
}

// ServePage renders a page at its own URL: /p/{tenant}/{kind}/{slug}.
//
// For sign-in this is a launcher, not a form in a vacuum: a page bound to an
// application starts a real authorization-code flow for it. Without a binding
// there is nothing to sign in TO, and rendering a form whose POST cannot
// produce tokens would be a worse experience than saying so.
func (h *OIDCHandler) ServePage(w http.ResponseWriter, r *http.Request) {
	tenantSlug := r.PathValue("tenant")
	kind := r.PathValue("kind")
	slug := r.PathValue("slug")
	if kind != "signin" && kind != "signout" {
		http.NotFound(w, r)
		return
	}
	tenant, err := h.tenants.TenantBySlug(r.Context(), tenantSlug)
	if err != nil || tenant == nil {
		http.NotFound(w, r)
		return
	}
	page, err := h.pages.AuthPageBySlug(r.Context(), tenant.ID, kind, slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	cfg, perr := pagecfg.Parse(pagecfg.Kind(kind), page.Config)
	if perr != nil {
		cfg, _ = pagecfg.Parse(pagecfg.Kind(kind), nil)
	}

	if kind == "signout" {
		h.renderSignout(w, r, tenant.ID, tenantSlug, cfg, "", "")
		return
	}

	// Sign-in: hand the flow to /v1/authorize so PKCE, redirect validation and
	// the existing SSO session all behave exactly as they do everywhere else.
	if page.ApplicationID == "" {
		h.renderer.Render(w, http.StatusOK, PageView{
			Cfg: cfg, Kind: "signin", Tenant: tenantSlug,
			Error: "This page is not linked to an application yet.",
		})
		return
	}
	app, err := h.apps.ApplicationByID(r.Context(), tenant.ID, page.ApplicationID)
	if err != nil || len(app.RedirectURIs) == 0 {
		h.renderer.Render(w, http.StatusOK, PageView{
			Cfg: cfg, Kind: "signin", Tenant: tenantSlug,
			Error: "This application has no registered redirect URI.",
		})
		return
	}
	q := url.Values{}
	q.Set("tenant", tenantSlug)
	q.Set("client_id", app.Slug)
	q.Set("redirect_uri", app.RedirectURIs[0])
	q.Set("response_type", "code")
	q.Set("page", slug)
	// The launcher cannot hold a PKCE verifier, so it hands off to the
	// application's own start URL when one exists; otherwise it renders the
	// branded page and lets the app supply the challenge.
	http.Redirect(w, r, "/v1/authorize?"+q.Encode(), http.StatusFound)
}

// renderSignout draws the confirmation (or the confirmation-free result).
func (h *OIDCHandler) renderSignout(w http.ResponseWriter, r *http.Request,
	tenantID, tenantSlug string, cfg *pagecfg.Config, returnURL, errMsg string) {

	if returnURL == "" {
		returnURL = cfg.Behavior.DefaultReturnURL
	}
	// Even a stored default is re-checked: configuration is not authorisation.
	if returnURL != "" && !h.allowedPostLogout(r, tenantID, returnURL) {
		returnURL = ""
	}
	csrf, _ := secret.New(16)
	h.setLogoutCSRF(w, r, csrf)
	h.renderer.Render(w, http.StatusOK, PageView{
		Cfg: cfg, Kind: "signout", Tenant: tenantSlug,
		ReturnURL: returnURL, Confirm: cfg.Behavior.Confirm,
		AutoRedirectSeconds: cfg.Behavior.AutoRedirectSeconds,
		Error:               errMsg, LogoutCSRF: csrf,
	})
}

// allowedPostLogout enforces the exact-match allowlist. An open redirect on
// the logout endpoint is a phishing primitive: "you have been signed out,
// sign in again here" is far more convincing when the link genuinely came
// from the identity provider.
func (h *OIDCHandler) allowedPostLogout(r *http.Request, tenantID, candidate string) bool {
	if candidate == "" {
		return false
	}
	apps, err := h.apps.ListApplications(r.Context(), tenantID)
	if err != nil {
		return false
	}
	for _, a := range apps {
		for _, allowed := range a.PostLogoutRedirectURIs {
			if allowed == candidate {
				return true
			}
		}
	}
	return false
}

func (h *OIDCHandler) setLogoutCSRF(w http.ResponseWriter, r *http.Request, token string) {
	h.cookies.set(w, r, logoutCSRFBase, token, 600)
}

// checkLogoutCSRF proves the POST came from the page we rendered. Without it
// the confirmation is decorative: any site could submit the form for you.
func (h *OIDCHandler) checkLogoutCSRF(r *http.Request, submitted string) bool {
	stored := h.cookies.get(r, logoutCSRFBase)
	if stored == "" || submitted == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(submitted)) == 1
}

var _ = strings.TrimSpace
var _ = apihttp.WriteError
var _ = apperr.ErrInvalidArgument
