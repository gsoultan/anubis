package authhttp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	apihttp "github.com/gsoultan/anubis/internal/api/http"
	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authapp "github.com/gsoultan/anubis/internal/auth/app"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/platform/ratelimit"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

const (
	authCodeTTL = 60 * time.Second
)

// OIDCHandler implements the browser SSO surface: the authorization code
// flow with PKCE, the hosted login page, and the code exchange. Wire shapes
// here are fixed by OIDC — that is why this lives on the stdlib mux.
type OIDCHandler struct {
	issuer        string
	tenants       tenancyport.TenantRepository
	realms        identityport.RealmRepository
	realmsAdmin   identityport.RealmAdminRepository
	ids           identityport.IdentityRepository
	creds         identityport.CredentialRepository
	sessions      authport.SessionRepository
	onetime       authport.OneTimeRepository
	apps          tenancyport.ApplicationRepository
	pages         tenancyport.AuthPageRepository
	refresh       authport.RefreshRepository
	renderer      *PageRenderer
	defaultTenant string
	issuerUC      authapp.TokenIssuer
	// cookies decides `__Host-`/Secure versus the development fallback; see
	// cookies.go for why that fallback exists and how narrow it is.
	cookies cookiePolicy
	clock   clock.Clock
	audit   auditport.Auditor
	limiter *ratelimit.Limiter
	logger  *slog.Logger
}

func NewOIDCHandler(
	issuer string,
	tenants tenancyport.TenantRepository,
	realms identityport.RealmRepository,
	realmsAdmin identityport.RealmAdminRepository,
	ids identityport.IdentityRepository,
	creds identityport.CredentialRepository,
	sessions authport.SessionRepository,
	onetime authport.OneTimeRepository,
	apps tenancyport.ApplicationRepository,
	pages tenancyport.AuthPageRepository,
	refresh authport.RefreshRepository,
	defaultTenant string,
	prod bool,
	issuerUC authapp.TokenIssuer,
	clock clock.Clock,
	audit auditport.Auditor,
	limiter *ratelimit.Limiter,
	logger *slog.Logger,
) *OIDCHandler {
	return &OIDCHandler{
		issuer: issuer, tenants: tenants, realms: realms, ids: ids,
		creds: creds, sessions: sessions, onetime: onetime, apps: apps,
		pages: pages, refresh: refresh, renderer: NewPageRenderer(),
		defaultTenant: defaultTenant, issuerUC: issuerUC,
		cookies: cookiePolicy{prod: prod}, clock: clock, audit: audit,
		limiter: limiter, logger: logger,
	}
}

// authCodePayload is the one_time_tokens payload for kind=auth_code.
type authCodePayload struct {
	TenantID            string `json:"tenant_id"`
	TenantSlug          string `json:"tenant_slug"`
	IdentityID          string `json:"identity_id"`
	SessionID           string `json:"session_id"`
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	Nonce               string `json:"nonce"`
}

// Authorize is GET /v1/authorize — the front door of browser SSO. A valid
// SSO cookie bounces straight back with a code, no prompt: that cookie IS
// the "single" in single sign-on.
func (h *OIDCHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tenantSlug := q.Get("tenant")
	if tenantSlug == "" {
		tenantSlug = "impack" // single-tenant default; multi-tenant callers pass ?tenant=
	}
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	state := q.Get("state")
	challenge := q.Get("code_challenge")
	method := q.Get("code_challenge_method")

	tenant, err := h.tenants.TenantBySlug(r.Context(), tenantSlug)
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrNotFound.With("tenant", tenantSlug))
		return
	}
	app, err := h.apps.ApplicationBySlug(r.Context(), tenant.ID, clientID)
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrInvalidArgument.With("client_id", "unknown"))
		return
	}
	// EXACT-match allowlist. No wildcards, no prefixes, no suffixes: open
	// redirect in an SSO service is full account takeover.
	if !exactMatch(app.RedirectURIs, redirectURI) {
		apihttp.WriteError(w, r, apperr.ErrRedirectURI)
		return
	}
	if q.Get("response_type") != "code" || challenge == "" || method != "S256" {
		h.redirectError(w, r, redirectURI, state, "invalid_request")
		return
	}

	// Existing SSO session?
	if raw := h.cookies.get(r, ssoCookieBase); raw != "" {
		if view, verr := h.sessions.SessionByCookieHash(r.Context(), secret.Hash(raw)); verr == nil && view.TenantID == tenant.ID {
			h.issueCode(w, r, tenant, view.IdentityID, view.ID, app.Slug, redirectURI, state, challenge, method, q.Get("nonce"))
			return
		}
	}
	h.renderLogin(w, r, tenant.ID, loginPageData{
		Tenant: tenantSlug, Realm: firstNonEmpty(q.Get("realm"), "internal"),
		ClientID: clientID, RedirectURI: redirectURI,
		State: state, Challenge: challenge, Method: method, Nonce: q.Get("nonce"),
		Page: q.Get("page"), ApplicationID: app.ID,
	})
}

// LoginForm is POST /v1/login — the hosted page submits here.
func (h *OIDCHandler) LoginForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		apihttp.WriteError(w, r, apperr.ErrInvalidArgument)
		return
	}
	tenantSlug := r.PostFormValue("tenant")
	username := r.PostFormValue("username")
	password := r.PostFormValue("password")
	realmCode := r.PostFormValue("realm")
	if realmCode == "" {
		realmCode = "internal"
	}

	ip := authctx.ClientIP(r.Context())
	if ok, retry := h.limiter.AllowAll(
		ratelimit.KeyLimit{Key: "ip:" + ip, Limit: ratelimit.Limit{PerMinute: 30, Burst: 30}},
		ratelimit.KeyLimit{Key: "acct:" + tenantSlug + "/" + realmCode + "/" + username,
			Limit: ratelimit.Limit{PerMinute: 10, Burst: 10}},
	); !ok {
		w.Header().Set("Retry-After", retry.String())
		apihttp.WriteError(w, r, apperr.ErrRateLimited)
		return
	}

	// Uniform-timing rule holds on the form path too.
	var identity *identitydomain.Identity
	var credential *credential.Credential
	var realm *identitydomain.Realm
	tenant, err := h.tenants.TenantBySlug(r.Context(), tenantSlug)
	if err == nil && tenant != nil {
		realm, err = h.realms.RealmByCode(r.Context(), tenant.ID, realmCode)
		if err == nil && realm != nil && realm.AllowsFactor("password") {
			identity, _ = h.ids.IdentityForLogin(r.Context(), tenant.ID, realm.ID, username)
			if identity != nil {
				credential, _ = h.creds.PasswordCredential(r.Context(), identity.ID)
			}
		}
	}
	hash := kdf.Dummy()
	if credential != nil && credential.Secret != "" {
		hash = credential.Secret
	}
	ok, _, kerr := kdf.Verify(password, hash)
	if kerr != nil || identity == nil || credential == nil || !ok ||
		identity.CanAuthenticate() != nil {
		h.renderLogin(w, r, tenantID(tenant), loginPageData{
			Tenant: tenantSlug, Realm: realmCode, ClientID: r.PostFormValue("client_id"),
			RedirectURI: r.PostFormValue("redirect_uri"), State: r.PostFormValue("state"),
			Challenge: r.PostFormValue("code_challenge"), Method: r.PostFormValue("code_challenge_method"),
			Nonce: r.PostFormValue("nonce"), Page: r.PostFormValue("page"),
			Error: "Invalid username or password",
		})
		return
	}

	// Browser session + __Host- cookie (Secure; Path=/; no Domain).
	sess, err := h.sessions.CreateSession(r.Context(), authdomain.SessionInput{
		IdentityID: identity.ID, TenantID: tenant.ID,
		AMR: []string{"pwd"}, IP: ip,
		UserAgent:    authctx.UserAgent(r.Context()),
		ActiveScopes: []byte("{}"),
		ExpiresAt:    h.clock.Now().Add(realm.SessionTTL),
	})
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return
	}
	cookieSecret, err := secret.New(32)
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return
	}
	if err := h.sessions.SetSessionCookieHash(r.Context(), sess.ID, secret.Hash(cookieSecret)); err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return
	}
	h.cookies.set(w, r, ssoCookieBase, cookieSecret, int(realm.SessionTTL/time.Second))
	h.audit.Emit(r.Context(), auditdomain.AuditEvent{
		TenantID: tenant.ID, ActorID: identity.ID, ActorKind: "identity",
		SessionID: sess.ID, Action: "auth.login", Result: "allow", IP: ip,
		Detail: []byte(`{"surface":"browser"}`),
	})
	h.issueCode(w, r, tenant, identity.ID, sess.ID,
		r.PostFormValue("client_id"), r.PostFormValue("redirect_uri"),
		r.PostFormValue("state"), r.PostFormValue("code_challenge"),
		r.PostFormValue("code_challenge_method"), r.PostFormValue("nonce"))
}

func (h *OIDCHandler) issueCode(w http.ResponseWriter, r *http.Request, tenant *tenancydomain.TenantRef, identityID, sessionID, clientID, redirectURI, state, challenge, method, nonce string) {
	// Re-validate redirect_uri against the app on EVERY code issue: the form
	// posts client-controlled fields back and must not be trusted.
	app, err := h.apps.ApplicationBySlug(r.Context(), tenant.ID, clientID)
	if err != nil || !exactMatch(app.RedirectURIs, redirectURI) {
		apihttp.WriteError(w, r, apperr.ErrRedirectURI)
		return
	}
	code, err := secret.New(32)
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return
	}
	payload, _ := json.Marshal(authCodePayload{
		TenantID: tenant.ID, TenantSlug: tenant.Slug, IdentityID: identityID,
		SessionID: sessionID, ClientID: clientID, RedirectURI: redirectURI,
		CodeChallenge: challenge, CodeChallengeMethod: method, Nonce: nonce,
	})
	if _, err := h.onetime.CreateOneTime(r.Context(), tenant.ID, "auth_code",
		secret.Hash(code), payload, h.clock.Now().Add(authCodeTTL)); err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return
	}
	u, _ := url.Parse(redirectURI)
	q := u.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// Token is POST /v1/token — the code exchange (single use, PKCE-verified).
func (h *OIDCHandler) Token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		apihttp.WriteError(w, r, apperr.ErrInvalidArgument)
		return
	}
	if r.PostFormValue("grant_type") != "authorization_code" {
		apihttp.WriteError(w, r, apperr.ErrInvalidArgument.With("grant_type", "authorization_code only"))
		return
	}
	code := r.PostFormValue("code")
	verifier := r.PostFormValue("code_verifier")
	if code == "" || verifier == "" {
		apihttp.WriteError(w, r, apperr.ErrPKCE)
		return
	}
	_, raw, err := h.onetime.ConsumeOneTime(r.Context(), "auth_code", secret.Hash(code))
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrPKCE)
		return
	}
	var p authCodePayload
	if json.Unmarshal(raw, &p) != nil {
		apihttp.WriteError(w, r, apperr.ErrPKCE)
		return
	}
	// PKCE S256: BASE64URL(SHA256(verifier)) must equal the stored challenge.
	sum := sha256.Sum256([]byte(verifier))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != p.CodeChallenge {
		apihttp.WriteError(w, r, apperr.ErrPKCE)
		return
	}
	// redirect_uri must repeat exactly (RFC 6749 §4.1.3).
	if r.PostFormValue("redirect_uri") != p.RedirectURI ||
		r.PostFormValue("client_id") != p.ClientID {
		apihttp.WriteError(w, r, apperr.ErrPKCE)
		return
	}
	view, err := h.sessions.SessionLive(r.Context(), p.SessionID)
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrSessionRevoked)
		return
	}
	pair, err := h.issuerUC.Issue(r.Context(), authapp.IssueInput{
		Session: view, TenantSlug: p.TenantSlug, ClientID: p.ClientID,
	})
	if err != nil {
		apihttp.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	apihttp.WriteJSON(w, http.StatusOK, map[string]any{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"token_type":    pair.TokenType,
		"expires_in":    pair.ExpiresIn,
		"session_id":    pair.SessionID,
	})
}

func (h *OIDCHandler) redirectError(w http.ResponseWriter, r *http.Request, redirectURI, state, code string) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrRedirectURI)
		return
	}
	q := u.Query()
	q.Set("error", code)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func exactMatch(allow []string, uri string) bool {
	if uri == "" {
		return false
	}
	for _, a := range allow {
		if a == uri {
			return true
		}
	}
	return false
}

func tenantID(t *tenancydomain.TenantRef) string {
	if t == nil {
		return ""
	}
	return t.ID
}

// ---------------------------------------------------------------------------
// Hosted login page. The page itself comes from the tenant's configured
// sign-in pages (migrations/0024) and is rendered by PageRenderer from a
// CONSTRAINED token set — never markup. Which page is chosen is decided by
// resolvePage: explicit ?page=, else the application's own page, else the
// tenant default.
// ---------------------------------------------------------------------------

// loginPageData is the flow state a sign-in page carries through its POST.
type loginPageData struct {
	Tenant, Realm, ClientID, RedirectURI, State, Challenge, Method, Nonce string
	// Page selects a specific sign-in page by slug; ApplicationID lets an
	// app-initiated flow keep its own branding.
	Page, ApplicationID string
	Error               string
}

// renderLogin draws the sign-in page for the current flow.
func (h *OIDCHandler) renderLogin(w http.ResponseWriter, r *http.Request, tenantID string, data loginPageData) {
	status := http.StatusOK
	if data.Error != "" {
		status = http.StatusUnauthorized
	}
	cfg := h.resolvePage(r, tenantID, "signin", data.Page, data.ApplicationID)

	view := PageView{
		Cfg: cfg, Kind: "signin",
		Tenant: data.Tenant, Realm: data.Realm, ClientID: data.ClientID,
		RedirectURI: data.RedirectURI, State: data.State,
		Challenge: data.Challenge, Method: data.Method, Nonce: data.Nonce,
		Error: data.Error,
	}
	// Only offer what the server will actually accept: a realm picker listing
	// realms that forbid passwords, or a registration link for a realm with
	// self-registration off, advertises doors that do not open.
	if tenantID != "" && cfg.Features.ShowRealmPicker {
		if realms, err := h.realmsAdmin.ListRealms(r.Context(), tenantID); err == nil {
			for _, rl := range realms {
				if containsString(rl.AllowedFactors, "password") {
					view.Realms = append(view.Realms, RealmChoice{Code: rl.Code, Name: rl.DisplayName})
				}
			}
		}
	}
	if tenantID != "" && cfg.Features.ShowRegistration && data.Realm != "" {
		if realm, err := h.realms.RealmByCode(r.Context(), tenantID, data.Realm); err == nil &&
			realm.SelfRegistration {
			view.RegistrationURL = "/p/" + data.Tenant + "/signin/" + firstNonEmpty(data.Page, "default") + "#register"
		}
	}
	h.renderer.Render(w, status, view)
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
