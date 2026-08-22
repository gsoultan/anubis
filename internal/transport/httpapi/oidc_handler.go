package httpapi

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/crypto/kdf"
	"github.com/gsoultan/anubis/internal/crypto/secret"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/ratelimit"
	"github.com/gsoultan/anubis/internal/repository"
	"github.com/gsoultan/anubis/internal/usecase"
)

const (
	ssoCookieName = "__Host-anubis_sso"
	authCodeTTL   = 60 * time.Second
)

// OIDCHandler implements the browser SSO surface: the authorization code
// flow with PKCE, the hosted login page, and the code exchange. Wire shapes
// here are fixed by OIDC — that is why this lives on the stdlib mux.
type OIDCHandler struct {
	issuer   string
	tenants  repository.TenantRepository
	realms   repository.RealmRepository
	ids      repository.IdentityRepository
	creds    repository.CredentialRepository
	sessions repository.SessionRepository
	onetime  repository.OneTimeRepository
	apps     repository.ApplicationRepository
	signin   repository.SigninPageRepository
	issuerUC usecase.TokenIssuer
	clock    repository.Clock
	audit    repository.Auditor
	limiter  *ratelimit.Limiter
	logger   *slog.Logger
}

func NewOIDCHandler(
	issuer string,
	tenants repository.TenantRepository,
	realms repository.RealmRepository,
	ids repository.IdentityRepository,
	creds repository.CredentialRepository,
	sessions repository.SessionRepository,
	onetime repository.OneTimeRepository,
	apps repository.ApplicationRepository,
	signin repository.SigninPageRepository,
	issuerUC usecase.TokenIssuer,
	clock repository.Clock,
	audit repository.Auditor,
	limiter *ratelimit.Limiter,
	logger *slog.Logger,
) *OIDCHandler {
	return &OIDCHandler{
		issuer: issuer, tenants: tenants, realms: realms, ids: ids,
		creds: creds, sessions: sessions, onetime: onetime, apps: apps,
		signin: signin, issuerUC: issuerUC, clock: clock, audit: audit,
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
		writeErr(w, r, domain.ErrNotFound.With("tenant", tenantSlug))
		return
	}
	app, err := h.apps.ApplicationBySlug(r.Context(), tenant.ID, clientID)
	if err != nil {
		writeErr(w, r, domain.ErrInvalidArgument.With("client_id", "unknown"))
		return
	}
	// EXACT-match allowlist. No wildcards, no prefixes, no suffixes: open
	// redirect in an SSO service is full account takeover.
	if !exactMatch(app.RedirectURIs, redirectURI) {
		writeErr(w, r, domain.ErrRedirectURI)
		return
	}
	if q.Get("response_type") != "code" || challenge == "" || method != "S256" {
		h.redirectError(w, r, redirectURI, state, "invalid_request")
		return
	}

	// Existing SSO session?
	if cookie, cerr := r.Cookie(ssoCookieName); cerr == nil {
		if view, verr := h.sessions.SessionByCookieHash(r.Context(), secret.Hash(cookie.Value)); verr == nil && view.TenantID == tenant.ID {
			h.issueCode(w, r, tenant, view.IdentityID, view.ID, app.Slug, redirectURI, state, challenge, method, q.Get("nonce"))
			return
		}
	}
	h.renderLogin(w, r, tenant.ID, loginPageData{
		Tenant: tenantSlug, ClientID: clientID, RedirectURI: redirectURI,
		State: state, Challenge: challenge, Method: method, Nonce: q.Get("nonce"),
	})
}

// LoginForm is POST /v1/login — the hosted page submits here.
func (h *OIDCHandler) LoginForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeErr(w, r, domain.ErrInvalidArgument)
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
		writeErr(w, r, domain.ErrRateLimited)
		return
	}

	// Uniform-timing rule holds on the form path too.
	var identity *domain.Identity
	var credential *repository.Credential
	var realm *domain.Realm
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
			Tenant: tenantSlug, ClientID: r.PostFormValue("client_id"),
			RedirectURI: r.PostFormValue("redirect_uri"), State: r.PostFormValue("state"),
			Challenge: r.PostFormValue("code_challenge"), Method: r.PostFormValue("code_challenge_method"),
			Nonce: r.PostFormValue("nonce"), Error: "Invalid username or password",
		})
		return
	}

	// Browser session + __Host- cookie (Secure; Path=/; no Domain).
	sess, err := h.sessions.CreateSession(r.Context(), repository.SessionInput{
		IdentityID: identity.ID, TenantID: tenant.ID,
		AMR: []string{"pwd"}, IP: ip,
		UserAgent:    authctx.UserAgent(r.Context()),
		ActiveScopes: []byte("{}"),
		ExpiresAt:    h.clock.Now().Add(realm.SessionTTL),
	})
	if err != nil {
		writeErr(w, r, domain.ErrInternal.Wrap(err))
		return
	}
	cookieSecret, err := secret.New(32)
	if err != nil {
		writeErr(w, r, domain.ErrInternal.Wrap(err))
		return
	}
	if err := h.sessions.SetSessionCookieHash(r.Context(), sess.ID, secret.Hash(cookieSecret)); err != nil {
		writeErr(w, r, domain.ErrInternal.Wrap(err))
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: ssoCookieName, Value: cookieSecret,
		Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		MaxAge: int(realm.SessionTTL / time.Second),
	})
	h.audit.Emit(r.Context(), repository.AuditEvent{
		TenantID: tenant.ID, ActorID: identity.ID, ActorKind: "identity",
		SessionID: sess.ID, Action: "auth.login", Result: "allow", IP: ip,
		Detail: []byte(`{"surface":"browser"}`),
	})
	h.issueCode(w, r, tenant, identity.ID, sess.ID,
		r.PostFormValue("client_id"), r.PostFormValue("redirect_uri"),
		r.PostFormValue("state"), r.PostFormValue("code_challenge"),
		r.PostFormValue("code_challenge_method"), r.PostFormValue("nonce"))
}

func (h *OIDCHandler) issueCode(w http.ResponseWriter, r *http.Request, tenant *repository.TenantRef, identityID, sessionID, clientID, redirectURI, state, challenge, method, nonce string) {
	// Re-validate redirect_uri against the app on EVERY code issue: the form
	// posts client-controlled fields back and must not be trusted.
	app, err := h.apps.ApplicationBySlug(r.Context(), tenant.ID, clientID)
	if err != nil || !exactMatch(app.RedirectURIs, redirectURI) {
		writeErr(w, r, domain.ErrRedirectURI)
		return
	}
	code, err := secret.New(32)
	if err != nil {
		writeErr(w, r, domain.ErrInternal.Wrap(err))
		return
	}
	payload, _ := json.Marshal(authCodePayload{
		TenantID: tenant.ID, TenantSlug: tenant.Slug, IdentityID: identityID,
		SessionID: sessionID, ClientID: clientID, RedirectURI: redirectURI,
		CodeChallenge: challenge, CodeChallengeMethod: method, Nonce: nonce,
	})
	if _, err := h.onetime.CreateOneTime(r.Context(), tenant.ID, "auth_code",
		secret.Hash(code), payload, h.clock.Now().Add(authCodeTTL)); err != nil {
		writeErr(w, r, domain.ErrInternal.Wrap(err))
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
		writeErr(w, r, domain.ErrInvalidArgument)
		return
	}
	if r.PostFormValue("grant_type") != "authorization_code" {
		writeErr(w, r, domain.ErrInvalidArgument.With("grant_type", "authorization_code only"))
		return
	}
	code := r.PostFormValue("code")
	verifier := r.PostFormValue("code_verifier")
	if code == "" || verifier == "" {
		writeErr(w, r, domain.ErrPKCE)
		return
	}
	_, raw, err := h.onetime.ConsumeOneTime(r.Context(), "auth_code", secret.Hash(code))
	if err != nil {
		writeErr(w, r, domain.ErrPKCE)
		return
	}
	var p authCodePayload
	if json.Unmarshal(raw, &p) != nil {
		writeErr(w, r, domain.ErrPKCE)
		return
	}
	// PKCE S256: BASE64URL(SHA256(verifier)) must equal the stored challenge.
	sum := sha256.Sum256([]byte(verifier))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != p.CodeChallenge {
		writeErr(w, r, domain.ErrPKCE)
		return
	}
	// redirect_uri must repeat exactly (RFC 6749 §4.1.3).
	if r.PostFormValue("redirect_uri") != p.RedirectURI ||
		r.PostFormValue("client_id") != p.ClientID {
		writeErr(w, r, domain.ErrPKCE)
		return
	}
	view, err := h.sessions.SessionLive(r.Context(), p.SessionID)
	if err != nil {
		writeErr(w, r, domain.ErrSessionRevoked)
		return
	}
	pair, err := h.issuerUC.Issue(r.Context(), usecase.IssueInput{
		Session: view, TenantSlug: p.TenantSlug, ClientID: p.ClientID,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
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
		writeErr(w, r, domain.ErrRedirectURI)
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

func tenantID(t *repository.TenantRef) string {
	if t == nil {
		return ""
	}
	return t.ID
}

// ---------------------------------------------------------------------------
// Hosted login page. Rendered from signin_pages.config — a CONSTRAINED token
// set (brand color, title, logo text), never arbitrary markup: the login page
// is the one page that must never break.
// ---------------------------------------------------------------------------

type loginPageData struct {
	Tenant, ClientID, RedirectURI, State, Challenge, Method, Nonce string
	Error                                                          string
	Title, Brand                                                   string
}

var loginTmpl = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}} — Sign in</title>
<style>
:root{--brand:{{.Brand}}}
body{font-family:system-ui,sans-serif;display:grid;place-items:center;min-height:100vh;margin:0;background:#f6f6f7}
form{background:#fff;padding:2rem;border-radius:12px;box-shadow:0 4px 24px rgba(0,0,0,.08);width:min(90vw,22rem)}
h1{font-size:1.25rem;margin:0 0 1rem}
label{display:block;font-size:.85rem;margin:.75rem 0 .25rem;color:#444}
input{width:100%;padding:.6rem;border:1px solid #ccc;border-radius:8px;box-sizing:border-box}
button{width:100%;margin-top:1.25rem;padding:.7rem;border:0;border-radius:8px;background:var(--brand);color:#fff;font-weight:600;cursor:pointer}
.err{color:#b00020;font-size:.85rem;margin-top:.75rem}
</style></head><body>
<form method="post" action="/v1/login">
<h1>Sign in to {{.Title}}</h1>
<input type="hidden" name="tenant" value="{{.Tenant}}">
<input type="hidden" name="client_id" value="{{.ClientID}}">
<input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
<input type="hidden" name="state" value="{{.State}}">
<input type="hidden" name="code_challenge" value="{{.Challenge}}">
<input type="hidden" name="code_challenge_method" value="{{.Method}}">
<input type="hidden" name="nonce" value="{{.Nonce}}">
<label for="u">Username</label>
<input id="u" name="username" autocomplete="username" required autofocus>
<label for="p">Password</label>
<input id="p" name="password" type="password" autocomplete="current-password" required>
{{if .Error}}<div class="err">{{.Error}}</div>{{end}}
<button type="submit">Sign in</button>
</form></body></html>`))

func (h *OIDCHandler) renderLogin(w http.ResponseWriter, r *http.Request, tenantID string, data loginPageData) {
	data.Title = "Anubis"
	data.Brand = "#4f46e5"
	if tenantID != "" {
		if cfg, _, err := h.signin.SigninPage(r.Context(), tenantID); err == nil {
			var page struct {
				Title string `json:"title"`
				Brand string `json:"brand_color"`
			}
			if json.Unmarshal(cfg, &page) == nil {
				if page.Title != "" {
					data.Title = page.Title
				}
				if validColor(page.Brand) {
					data.Brand = page.Brand
				}
			}
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if data.Error != "" {
		w.WriteHeader(http.StatusUnauthorized)
	}
	_ = loginTmpl.Execute(w, data)
}

// validColor allows only #rgb/#rrggbb — the config must not become a CSS
// injection vector on the one page that must never break.
func validColor(s string) bool {
	if len(s) != 4 && len(s) != 7 {
		return false
	}
	if s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
