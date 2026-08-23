package authhttp

import (
	"net/http"

	apihttp "github.com/gsoultan/anubis/internal/api/http"
	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/tenancy/domain/pagecfg"
)

// LogoutPage is GET /v1/logout — RP-initiated logout, OIDC-shaped.
//
// It renders the tenant's sign-out page and asks first. The confirmation is
// not politeness: GET /v1/logout is reachable from any page on the internet
// (an <img> tag is enough), so without it a third party can end sessions at
// will. Answering with a form makes ending the session a deliberate act.
func (h *OIDCHandler) LogoutPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tenantSlug := q.Get("tenant")
	if tenantSlug == "" {
		tenantSlug = h.defaultTenant
	}
	tenant, err := h.tenants.TenantBySlug(r.Context(), tenantSlug)
	if err != nil || tenant == nil {
		apihttp.WriteError(w, r, apperr.ErrNotFound.With("tenant", tenantSlug))
		return
	}
	cfg := h.resolvePage(r, tenant.ID, "signout", q.Get("page"), "")

	returnURL := q.Get("post_logout_redirect_uri")
	var errMsg string
	if returnURL != "" && !h.allowedPostLogout(r, tenant.ID, returnURL) {
		// Refuse the redirect, still sign out. Telling the user their return
		// link was rejected beats silently sending them somewhere unexpected.
		errMsg = "The requested return address is not registered for this tenant."
		returnURL = ""
	}
	if !cfg.Behavior.Confirm {
		h.performLogout(w, r, tenant.ID, tenantSlug, cfg, returnURL)
		return
	}
	h.renderSignout(w, r, tenant.ID, tenantSlug, cfg, returnURL, errMsg)
}

// LogoutSubmit is POST /v1/logout — the confirmed sign-out.
func (h *OIDCHandler) LogoutSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		apihttp.WriteError(w, r, apperr.ErrInvalidArgument)
		return
	}
	tenantSlug := r.PostFormValue("tenant")
	if tenantSlug == "" {
		tenantSlug = h.defaultTenant
	}
	tenant, err := h.tenants.TenantBySlug(r.Context(), tenantSlug)
	if err != nil || tenant == nil {
		apihttp.WriteError(w, r, apperr.ErrNotFound.With("tenant", tenantSlug))
		return
	}
	cfg := h.resolvePage(r, tenant.ID, "signout", r.PostFormValue("page"), "")

	// The confirmation is only worth something if the POST proves it came
	// from the page we rendered.
	if cfg.Behavior.Confirm && !h.checkLogoutCSRF(r, r.PostFormValue("csrf")) {
		h.renderSignout(w, r, tenant.ID, tenantSlug, cfg, "",
			"That sign-out request expired. Please try again.")
		return
	}
	returnURL := r.PostFormValue("post_logout_redirect_uri")
	if returnURL != "" && !h.allowedPostLogout(r, tenant.ID, returnURL) {
		returnURL = ""
	}
	h.performLogout(w, r, tenant.ID, tenantSlug, cfg, returnURL)
}

// performLogout revokes the browser session behind the SSO cookie, clears the
// cookie, and renders the signed-out state.
func (h *OIDCHandler) performLogout(w http.ResponseWriter, r *http.Request,
	tenantID, tenantSlug string, cfg *pagecfg.Config, returnURL string) {

	if raw := h.cookies.get(r, ssoCookieBase); raw != "" {
		if view, verr := h.sessions.SessionByCookieHash(r.Context(), secret.Hash(raw)); verr == nil {
			if _, rerr := h.sessions.RevokeSession(r.Context(), view.TenantID, view.ID, "rp_initiated_logout"); rerr == nil {
				_, _ = h.refresh.RevokeRefreshBySessions(r.Context(), []string{view.ID})
			}
			h.audit.Emit(r.Context(), auditdomain.AuditEvent{
				TenantID: view.TenantID, ActorID: view.IdentityID, ActorKind: "identity",
				SessionID: view.ID, Action: "auth.logout", Result: "allow",
				IP: authctx.ClientIP(r.Context()), Detail: []byte(`{"surface":"rp_initiated"}`),
			})
		}
	}
	// Clear the cookie whether or not a session was found: a stale cookie on
	// the browser is worse than none.
	h.cookies.clear(w, r, ssoCookieBase)
	h.cookies.clear(w, r, logoutCSRFBase)

	// An immediate redirect is only safe because returnURL passed the
	// allowlist above; auto-redirect happens on the page instead, so the user
	// sees they were signed out.
	if returnURL != "" && cfg.Behavior.AutoRedirectSeconds == 0 && !cfg.Behavior.Confirm {
		http.Redirect(w, r, returnURL, http.StatusFound)
		return
	}
	h.renderer.Render(w, http.StatusOK, PageView{
		Cfg: cfg, Kind: "signout", Tenant: tenantSlug,
		ReturnURL: returnURL, SignedOut: true,
		AutoRedirectSeconds: cfg.Behavior.AutoRedirectSeconds,
	})
}
