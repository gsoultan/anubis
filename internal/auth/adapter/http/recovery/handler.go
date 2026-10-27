// Package authhttprecovery serves the hosted password-reset pages:
//
//	GET  /p/{tenant}/reset/request    ask for a link
//	POST /p/{tenant}/reset/request    send it (always answers "check your email")
//	GET  /p/{tenant}/reset/complete   the page the link opens
//	POST /p/{tenant}/reset/complete   set the password
//
// They render through the tenant's sign-in page configuration, so they carry
// its brand without a configuration of their own. No CSRF cookie: the
// completion form is authorised by the single-use token it carries, and the
// request form takes a username only and is rate limited, so a cross-site
// submission can at worst send somebody a link they did not ask for.
package authhttprecovery

import (
	"log/slog"
	"net/http"
	"net/url"

	apihttp "github.com/gsoultan/anubis/internal/api/http"
	authhttp "github.com/gsoultan/anubis/internal/auth/adapter/http"
	identityrecovery "github.com/gsoultan/anubis/internal/identity/app/recovery"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/ratelimit"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/validate"
	"github.com/gsoultan/anubis/internal/tenancy/domain/pagecfg"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// Handler is the hosted reset flow.
type Handler struct {
	svc      identityrecovery.RecoveryUsecase
	tenants  tenancyport.TenantRepository
	realms   identityport.RealmRepository
	pages    tenancyport.AuthPageRepository
	renderer *authhttp.PageRenderer
	limiter  *ratelimit.Limiter
	logger   *slog.Logger
}

func NewHandler(
	svc identityrecovery.RecoveryUsecase,
	tenants tenancyport.TenantRepository,
	realms identityport.RealmRepository,
	pages tenancyport.AuthPageRepository,
	limiter *ratelimit.Limiter,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		svc: svc, tenants: tenants, realms: realms, pages: pages,
		renderer: authhttp.NewPageRenderer(), limiter: limiter, logger: logger,
	}
}

// Mount registers the four routes. The literal "reset" segment is more
// specific than ServePage's /p/{tenant}/{kind}/{slug}, so the mux prefers
// these without ServePage needing to know.
func (h *Handler) Mount(handle func(pattern string, handler http.HandlerFunc)) {
	handle("GET /p/{tenant}/reset/request", h.RequestPage)
	handle("POST /p/{tenant}/reset/request", h.RequestSubmit)
	handle("GET /p/{tenant}/reset/complete", h.CompletePage)
	handle("POST /p/{tenant}/reset/complete", h.CompleteSubmit)
}

// page is what every render here shares: the tenant's sign-in page config
// and the realm it is for. A tenant that does not exist still gets a page —
// defaults — because a 404 on the reset URL says which slugs are real.
type page struct {
	tenantSlug string
	tenantID   string
	realm      string
	minLength  int
	cfg        *pagecfg.Config
}

func (h *Handler) resolve(r *http.Request) page {
	p := page{
		tenantSlug: r.PathValue("tenant"),
		realm:      r.FormValue("realm"),
		minLength:  identitydomain.ParsePasswordPolicy(nil).MinLength,
	}
	if p.realm == "" || !validate.ValidCode(p.realm) {
		p.realm = "internal"
	}
	if !validate.ValidSlug(p.tenantSlug) {
		p.cfg, _ = pagecfg.Parse(pagecfg.KindSignin, nil)
		return p
	}
	ctx := r.Context()
	var raw []byte
	if tenant, err := h.tenants.TenantBySlug(ctx, p.tenantSlug); err == nil && tenant != nil {
		p.tenantID = tenant.ID
		if realm, err := h.realms.RealmByCode(ctx, tenant.ID, p.realm); err == nil && realm != nil {
			p.minLength = realm.PasswordPolicy.MinLength
			if pg, err := h.pages.AuthPageForRealm(ctx, tenant.ID, "signin", realm.ID); err == nil && pg != nil {
				raw = pg.Config
			}
		}
		if raw == nil {
			if pg, err := h.pages.DefaultAuthPage(ctx, tenant.ID, "signin"); err == nil && pg != nil {
				raw = pg.Config
			}
		}
	}
	cfg, err := pagecfg.Parse(pagecfg.KindSignin, raw)
	if err != nil {
		h.logger.Error("stored page config is invalid; rendering defaults",
			"tenant", p.tenantID, "kind", "signin", "error", err)
		cfg, _ = pagecfg.Parse(pagecfg.KindSignin, nil)
	}
	p.cfg = cfg
	return p
}

func (h *Handler) render(w http.ResponseWriter, status int, p page, step, title, errMsg, token string) {
	q := url.Values{"realm": {p.realm}}
	h.renderer.Render(w, status, authhttp.PageView{
		Cfg: p.cfg, Kind: "reset", Tenant: p.tenantSlug, Realm: p.realm,
		Title: title, ResetStep: step, ResetToken: token, MinLength: p.minLength,
		Error:     errMsg,
		SigninURL: "/p/" + p.tenantSlug + "/signin/default?" + q.Encode(),
		ResetURL:  "/p/" + p.tenantSlug + "/reset/request?" + q.Encode(),
	})
}

func (h *Handler) RequestPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, http.StatusOK, h.resolve(r), "request", "Reset your password", "", "")
}

func (h *Handler) RequestSubmit(w http.ResponseWriter, r *http.Request) {
	p := h.resolve(r)
	username := r.PostFormValue("username")
	// The same budget the RPC gets: a mail per accepted request, so three a
	// minute to one account and ten from one address.
	ip := authctx.ClientIP(r.Context())
	if ok, retry := h.limiter.AllowAll(
		ratelimit.KeyLimit{Key: "ip:" + ip, Limit: ratelimit.Limit{PerMinute: 10, Burst: 10}},
		ratelimit.KeyLimit{Key: "acct:" + p.tenantSlug + "/" + p.realm + "/" + username,
			Limit: ratelimit.Limit{PerMinute: 3, Burst: 3}},
	); !ok {
		w.Header().Set("Retry-After", retry.String())
		apihttp.WriteError(w, r, apperr.ErrRateLimited)
		return
	}
	err := h.svc.RequestPasswordReset(r.Context(), identityrecovery.RequestResetInput{
		Tenant: p.tenantSlug, Realm: p.realm, Username: username,
	})
	if err != nil {
		// The one error the usecase returns is true for everybody: this
		// server cannot send mail. Saying so is not an enumeration.
		h.render(w, http.StatusServiceUnavailable, p, "request", "Reset your password",
			"This server cannot send email right now. Please ask an administrator to reset your password.", "")
		return
	}
	h.render(w, http.StatusOK, p, "sent", "Check your email", "", "")
}

func (h *Handler) CompletePage(w http.ResponseWriter, r *http.Request) {
	p := h.resolve(r)
	token := r.URL.Query().Get("token")
	if token == "" || len(token) > 128 {
		h.render(w, http.StatusBadRequest, p, "expired", "This link is not valid",
			"The link is incomplete. Open it from the email, or request a new one.", "")
		return
	}
	// The token is moved from the URL into the form and nothing is checked
	// yet: checking would consume it, and a mail client that prefetches
	// links would burn every reset before the person saw the page.
	h.render(w, http.StatusOK, p, "complete", "Choose a new password", "", token)
}

func (h *Handler) CompleteSubmit(w http.ResponseWriter, r *http.Request) {
	p := h.resolve(r)
	token := r.PostFormValue("token")
	err := h.svc.CompletePasswordReset(r.Context(), token, r.PostFormValue("password"))
	switch {
	case err == nil:
		h.render(w, http.StatusOK, p, "done", "Your password is set", "", "")
	case apperr.AsError(err).Code == "password_policy":
		// The token was consumed with the attempt. Honest about it: the form
		// cannot be resubmitted, so the page offers a new link rather than
		// a field to retry.
		h.render(w, http.StatusBadRequest, p, "expired", "That password was not accepted",
			"Passwords need at least "+itoa(p.minLength)+" characters. The link has been used; please request a new one.", "")
	case apperr.AsError(err).Code == "invalid_token":
		h.render(w, http.StatusBadRequest, p, "expired", "This link has expired",
			"Reset links work once, within 30 minutes. Request a new one to continue.", "")
	default:
		h.render(w, http.StatusBadRequest, p, "expired", "The password could not be set",
			apperr.AsError(err).Message, "")
	}
}

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
