package authhttp

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	apihttp "github.com/gsoultan/anubis/internal/api/http"
	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authapp "github.com/gsoultan/anubis/internal/auth/app"
	"github.com/gsoultan/anubis/internal/auth/app/enroll"
	"github.com/gsoultan/anubis/internal/auth/app/signin"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/platform/crypto/totp"
	"github.com/gsoultan/anubis/internal/platform/ratelimit"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
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
	issuer string
	// auth decides whether a password gets in. It is the SAME instance the
	// Connect login service uses: this page is the second door onto one
	// identity, and a policy it evaluated for itself was a policy it got
	// wrong twice.
	auth          *signin.PasswordAuthenticator
	granter       *signin.EnrolmentGranter
	enrolment     enroll.EnrollmentUsecase
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
	// ring unseals TOTP secrets for the browser second-factor step.
	ring *keyring.Manager
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
	auth *signin.PasswordAuthenticator,
	granter *signin.EnrolmentGranter,
	enrolment enroll.EnrollmentUsecase,
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
	ring *keyring.Manager,
	clock clock.Clock,
	audit auditport.Auditor,
	limiter *ratelimit.Limiter,
	logger *slog.Logger,
) *OIDCHandler {
	return &OIDCHandler{
		issuer: issuer, auth: auth, granter: granter, enrolment: enrolment,
		tenants: tenants, realms: realms,
		realmsAdmin: realmsAdmin, ids: ids,
		creds: creds, sessions: sessions, onetime: onetime, apps: apps,
		pages: pages, refresh: refresh, renderer: NewPageRenderer(),
		defaultTenant: defaultTenant, issuerUC: issuerUC, ring: ring,
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

	// Before anything else, including the rate limiter: a submission that
	// cannot have come from our page is not a login attempt, and counting it
	// as one lets a cross-site form burn a victim's account budget.
	if !h.checkLoginCSRF(r, r.PostFormValue("csrf")) {
		h.renderLogin(w, r, "", loginPageData{
			Tenant: tenantSlug, Realm: realmCode,
			Error: "That form expired. Please try again.",
		})
		return
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

	// A second submit carries the single-use token this server minted for a
	// password it already checked, and NO password — the rendered form has no
	// field for one, and its comment says why: "the password is not asked for
	// again, and nothing on this page can replay it". Asking for it anyway
	// made the token stand for nothing, and the hosted second-factor step
	// could not be completed by a browser at all.
	if mfaTok := r.PostFormValue("mfa_token"); mfaTok != "" {
		h.completeSecondFactor(w, r, tenantSlug, realmCode, mfaTok, r.PostFormValue("code"))
		return
	}
	// Same shape for the enrolment step: the handle stands for the password,
	// and the form carries no credentials to re-check.
	if enrolTok := r.PostFormValue("enrol_token"); enrolTok != "" {
		h.completeEnrolment(w, r, tenantSlug, realmCode, enrolTok, r.PostFormValue("code"))
		return
	}
	// "Set it up now" on the grace-period warning. No token stands for the
	// password here because a session already does: this member is signed in,
	// and the SSO cookie is the proof.
	if r.PostFormValue("enrol_now") != "" {
		h.enrolFromSession(w, r, tenantSlug, realmCode)
		return
	}

	// Every property of a password sign-in — uniform failure timing, the
	// realm's allowed factors, a blocked identity, the KDF rehash, the deny
	// audit, an enrolled second factor and the realm's enrolment deadline — is
	// decided by the authenticator the API door also calls. This handler used
	// to re-derive all of it, and got two of them wrong.
	d := h.auth.Authenticate(r.Context(), signin.LoginInput{
		Tenant: tenantSlug, Realm: realmCode,
		Username: username, Password: password,
		ClientID: r.PostFormValue("client_id"),
	}, signin.SurfaceBrowser)

	switch d.Step {
	case signin.StepDeny:
		// One message for every credential refusal, as on the API door:
		// which of them it was is precisely what an attacker is asking.
		// An internal failure is the exception — telling somebody their
		// password is wrong when the server could not check it sends them
		// to reset a password that was fine.
		msg := "Invalid username or password"
		if apperr.AsError(d.Err).Code == apperr.ErrInternal.Code {
			msg = "Sign-in is temporarily unavailable. Please try again."
		}
		h.renderLogin(w, r, tenantID(d.Tenant),
			h.resubmit(r, tenantSlug, realmCode, msg))
		return

	case signin.StepEnrol:
		// The realm requires a factor this member never enrolled and the date
		// for enrolling it has passed. No session — and a way to comply, or
		// the policy is unsatisfiable by exactly the people it applies to.
		h.promptForEnrolment(w, r, tenantSlug, realmCode, d, enrolmentMessage(d.Missing), "")
		return
	}

	// FIRST submit on an identity that holds a factor: ask for it instead of
	// minting anything. No session, no cookie, no code. This page used to go
	// straight from password to session while AuthService.Login refused the
	// same sign-in — a factor belongs to the IDENTITY, not to the door, and
	// now neither door decides that for itself.
	if d.Step == signin.StepFactor {
		h.promptForFactor(w, r, d.Tenant, d.Identity, d.Realm, "")
		return
	}

	if d.Due {
		// Signed in, and told why that will stop working. This is the only
		// moment the browser gets: the ordinary path redirects straight back
		// to the application, which is why a member who only uses SSO used to
		// meet an enrol-or-deny policy for the first time on the day it
		// refused them.
		h.warnEnrolmentDue(w, r, tenantSlug, realmCode, d)
		return
	}
	h.establishBrowserSession(w, r, d.Tenant, d.Realm, d.Identity, []string{"pwd"}, "password")
}

// completeSecondFactor finishes a sign-in whose password was checked on an
// earlier request.
//
// Everything it needs comes from the single-use token rather than from the
// form: the identity id is inside a value only this server could have
// written, so a submission cannot name a different one, and ConsumeOneTime
// makes it good exactly once. That is what lets the form carry no password —
// and what makes re-asking for one a bug rather than defence in depth.
func (h *OIDCHandler) completeSecondFactor(w http.ResponseWriter, r *http.Request, tenantSlug, realmCode, token, code string) {
	restart := func(msg string) {
		h.renderLogin(w, r, "", h.resubmit(r, tenantSlug, realmCode, msg))
	}
	tenant, err := h.tenants.TenantBySlug(r.Context(), tenantSlug)
	if err != nil || tenant == nil {
		restart("Please sign in again.")
		return
	}
	realm, err := h.realms.RealmByCode(r.Context(), tenant.ID, realmCode)
	if err != nil || realm == nil {
		restart("Please sign in again.")
		return
	}
	identityID, err := h.consumeFactorToken(r, token)
	if err != nil {
		// Expired, already spent, or never ours. There is nothing left to
		// retry against, so the only honest answer is to start over.
		restart("That took too long. Please sign in again.")
		return
	}
	identity, err := h.ids.Identity(r.Context(), tenant.ID, identityID)
	if err != nil || identity == nil {
		restart("Please sign in again.")
		return
	}
	// Re-checked here, not merely at the password step: an identity can be
	// blocked between the two submits, and this one issues the session.
	if identity.CanAuthenticate() != nil {
		h.auth.AuditLogin(r.Context(), tenant, identity, signin.SurfaceBrowser,
			"deny", "identity_cannot_authenticate", "")
		restart("Please sign in again.")
		return
	}
	if err := h.verifyFactorCode(r, identity, code); err != nil {
		h.audit.Emit(r.Context(), auditdomain.AuditEvent{
			TenantID: tenant.ID, ActorID: identity.ID, ActorKind: "identity",
			Action: "auth.mfa", Result: "deny", IP: authctx.ClientIP(r.Context()),
			Detail: []byte(`{"surface":"browser","method":"totp"}`),
		})
		// A rejected code must leave a way to try again, or one typo means
		// restarting the whole sign-in.
		h.promptForFactor(w, r, tenant, identity, realm, "That code was not accepted.")
		return
	}
	h.establishBrowserSession(w, r, tenant, realm, identity, []string{"pwd", "otp"}, "mfa")
}

// establishBrowserSession is the end of every successful hosted sign-in:
// session row, __Host- cookie (Secure; Path=/; no Domain), audit, code.
func (h *OIDCHandler) establishBrowserSession(
	w http.ResponseWriter, r *http.Request,
	tenant *tenancydomain.TenantRef, realm *identitydomain.Realm,
	identity *identitydomain.Identity, amr []string, detail string,
) {
	sessionID := h.openBrowserSession(w, r, tenant, realm, identity, amr, detail)
	if sessionID == "" {
		return
	}
	h.issueCode(w, r, tenant, identity.ID, sessionID,
		r.PostFormValue("client_id"), r.PostFormValue("redirect_uri"),
		r.PostFormValue("state"), r.PostFormValue("code_challenge"),
		r.PostFormValue("code_challenge_method"), r.PostFormValue("nonce"))
}

// openBrowserSession creates the session row, sets the __Host- cookie and
// writes the audit event. It returns the session id, or "" when it has
// already written an error response.
//
// Separate from issuing the code because one caller has something to say
// first: a member inside the grace period IS signed in, and the page that
// tells them so is the only chance the browser gets to mention the deadline.
func (h *OIDCHandler) openBrowserSession(
	w http.ResponseWriter, r *http.Request,
	tenant *tenancydomain.TenantRef, realm *identitydomain.Realm,
	identity *identitydomain.Identity, amr []string, detail string,
) string {
	sess, err := h.sessions.CreateSession(r.Context(), authdomain.SessionInput{
		IdentityID: identity.ID, TenantID: tenant.ID,
		AMR: amr, IP: authctx.ClientIP(r.Context()),
		UserAgent:    authctx.UserAgent(r.Context()),
		ActiveScopes: []byte("{}"),
		ExpiresAt:    h.clock.Now().Add(realm.SessionTTL),
	})
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return ""
	}
	cookieSecret, err := secret.New(32)
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return ""
	}
	if err := h.sessions.SetSessionCookieHash(r.Context(), sess.ID, secret.Hash(cookieSecret)); err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return ""
	}
	h.cookies.set(w, r, ssoCookieBase, cookieSecret, int(realm.SessionTTL/time.Second))
	h.auth.AuditLogin(r.Context(), tenant, identity, signin.SurfaceBrowser,
		"allow", detail, sess.ID)
	return sess.ID
}

// resubmit rebuilds the page data from the flow fields the form posted back,
// so a re-render keeps the caller's OIDC parameters instead of dropping them
// and stranding the sign-in.
func (h *OIDCHandler) resubmit(r *http.Request, tenantSlug, realmCode, msg string) loginPageData {
	return loginPageData{
		Tenant: tenantSlug, Realm: realmCode,
		ClientID:    r.PostFormValue("client_id"),
		RedirectURI: r.PostFormValue("redirect_uri"),
		State:       r.PostFormValue("state"),
		Challenge:   r.PostFormValue("code_challenge"),
		Method:      r.PostFormValue("code_challenge_method"),
		Nonce:       r.PostFormValue("nonce"),
		Page:        r.PostFormValue("page"),
		Error:       msg,
	}
}

// enrolmentMessage says which factor has to exist before this account signs
// in again. The realm stores factor KINDS, which are fine in a policy and
// useless on a page: "totp" is not a thing anybody outside this system has
// heard of.
func enrolmentMessage(missing []string) string {
	if len(missing) == 0 {
		return "This realm requires a second factor that this account does not have."
	}
	return "This realm requires " + factorNames(missing) +
		", which this account has not set up. Enrol one, then sign in again."
}

// factorNames renders factor KINDS as something a person has heard of.
// "totp" is fine in a policy and useless on a page.
func factorNames(missing []string) string {
	names := make([]string, 0, len(missing))
	for _, m := range missing {
		names = append(names, factorName(m))
	}
	if len(names) == 0 {
		return "a second factor"
	}
	return strings.Join(names, " and ")
}

func factorName(kind string) string {
	switch kind {
	case "totp":
		return "an authenticator app"
	case "device_key":
		return "a device passkey"
	}
	return kind
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
	// MFAToken turns the page into the second-factor step. Single use, and
	// it stands for a password that has already been verified.
	MFAToken string
	// EnrolToken turns the page into the enrolment step: an opaque single-use
	// handle for a pending enrolment this server is driving. The grant it was
	// minted against never reaches the browser.
	EnrolToken string
	EnrolKey   string
	EnrolURI   string
	// RecoveryCodes are shown exactly once, above the sign-in form, because
	// that is the only moment they exist in readable form.
	RecoveryCodes []string
	// WarnDeadline turns the page into the grace-period warning: signed in,
	// and this is the date it stops working. WarnFactors names what to enrol.
	WarnDeadline string
	WarnFactors  string
	// ContinueURL re-enters /v1/authorize, which finds the SSO cookie and
	// issues the code. Set whenever the page is shown to somebody who is
	// already signed in.
	ContinueURL string
}

// loginCSRFTTL bounds how long a rendered sign-in page stays submittable.
// Long enough to read a password manager, short enough that a page left open
// overnight does not carry a usable token into the morning.
const loginCSRFTTL = 30 * time.Minute

// renderLogin draws the sign-in page for the current flow.
func (h *OIDCHandler) renderLogin(w http.ResponseWriter, r *http.Request, tenantID string, data loginPageData) {
	status := http.StatusOK
	if data.Error != "" {
		status = http.StatusUnauthorized
	}
	cfg := h.resolvePageForRealm(r, tenantID, "signin", data.Page, data.ApplicationID, data.Realm)

	// A fresh token per render, cookie and form together. The MFA step
	// re-renders through here too, so its submission carries one as well.
	csrf, err := secret.New(16)
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return
	}
	h.cookies.set(w, r, loginCSRFBase, csrf, int(loginCSRFTTL/time.Second))

	view := PageView{
		Cfg: cfg, Kind: "signin", LoginCSRF: csrf,
		EnrolToken: data.EnrolToken, EnrolKey: data.EnrolKey,
		EnrolURI: template.URL(data.EnrolURI), RecoveryCodes: data.RecoveryCodes,
		WarnDeadline: data.WarnDeadline, WarnFactors: data.WarnFactors,
		ContinueURL: data.ContinueURL,
		Tenant:      data.Tenant, Realm: data.Realm, ClientID: data.ClientID,
		RedirectURI: data.RedirectURI, State: data.State,
		Challenge: data.Challenge, Method: data.Method, Nonce: data.Nonce,
		Error: data.Error, MFAToken: data.MFAToken,
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

// browserFactorTTL bounds the gap between the password and the code. Long
// enough to open an authenticator app, short enough that a token left in a
// browser tab is not a standing credential.
const browserFactorTTL = 5 * time.Minute

// promptForFactor mints the single-use token standing for the verified
// password and re-renders the page asking for a code.
//
// The token carries the identity, so the second submit cannot name a
// different one: everything the second step needs is inside a value only
// this server can have written.
func (h *OIDCHandler) promptForFactor(
	w http.ResponseWriter, r *http.Request,
	tenant *tenancydomain.TenantRef, identity *identitydomain.Identity,
	realm *identitydomain.Realm, msg string,
) {
	raw, err := secret.New(32)
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return
	}
	// payload is jsonb, so it has to BE json — a bare uuid is not, and the
	// insert fails with a 500 that says nothing about why.
	if _, err := h.onetime.CreateOneTime(r.Context(), tenant.ID, "browser_mfa",
		secret.Hash(raw), jsonx.Must(map[string]string{"identity_id": identity.ID}),
		h.clock.Now().Add(browserFactorTTL)); err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return
	}
	h.renderLogin(w, r, tenant.ID, loginPageData{
		Tenant: r.PostFormValue("tenant"), Realm: realmCodeOf(realm),
		ClientID: r.PostFormValue("client_id"), RedirectURI: r.PostFormValue("redirect_uri"),
		State: r.PostFormValue("state"), Challenge: r.PostFormValue("code_challenge"),
		Method: r.PostFormValue("code_challenge_method"), Nonce: r.PostFormValue("nonce"),
		Page: r.PostFormValue("page"), Error: msg, MFAToken: raw,
	})
}

// consumeFactorToken spends the single-use token and returns the identity it
// was minted for.
//
// ConsumeOneTime is atomic, so the token cannot be spent twice. The identity
// comes from the token's payload rather than from the form: the token names
// the identity whose password was checked, and a submission arriving with
// somebody else's username must not be able to borrow it.
func (h *OIDCHandler) consumeFactorToken(r *http.Request, token string) (string, error) {
	_, payload, err := h.onetime.ConsumeOneTime(r.Context(), "browser_mfa", secret.Hash(token))
	if err != nil {
		return "", apperr.ErrMfaInvalid
	}
	var claim struct {
		IdentityID string `json:"identity_id"`
	}
	if err := json.Unmarshal(payload, &claim); err != nil || claim.IdentityID == "" {
		return "", apperr.ErrMfaInvalid
	}
	return claim.IdentityID, nil
}

// verifyFactorCode checks a TOTP code for an identity.
//
// The code goes through AdvanceCredentialStep — the same guard the API path
// uses, which is in the database rather than in Go, so two presentations of
// one code cannot both win.
func (h *OIDCHandler) verifyFactorCode(r *http.Request, identity *identitydomain.Identity, code string) error {
	cred, err := h.creds.ActiveCredentialOfKind(r.Context(), identity.ID, "totp")
	if err != nil || cred == nil {
		return apperr.ErrMfaInvalid
	}
	sealed, err := base64.RawStdEncoding.DecodeString(cred.Secret)
	if err != nil {
		return apperr.ErrMfaInvalid
	}
	shared, _, err := keyring.OpenNamedSecret(h.ring.Ring(), cred.SecretKid, "totp:"+cred.ID, sealed)
	if err != nil {
		return apperr.ErrMfaInvalid
	}
	step, ok := totp.Verify(shared, code, h.clock.Now(), totp.DefaultStep, totp.DefaultDigits, 1)
	if !ok {
		return apperr.ErrMfaInvalid
	}
	fresh, err := h.creds.AdvanceCredentialStep(r.Context(), cred.ID, step)
	if err != nil || !fresh {
		return apperr.ErrMfaInvalid
	}
	return nil
}

func realmCodeOf(realm *identitydomain.Realm) string {
	if realm == nil {
		return ""
	}
	return realm.Code
}

// checkLoginCSRF proves a sign-in submission came from a page this server
// rendered.
//
// Logout has had this since it was written — "without it the confirmation is
// decorative: any site could submit the form for you" — and login did not,
// with no Origin or Referer check either. So any page could auto-submit a
// form carrying the ATTACKER's credentials and leave the visitor holding an
// SSO cookie for somebody else's account. SameSite=Lax does not help:
// SameSite decides whether a cookie is SENT, not whether one can be SET.
//
// On an identity provider that is worse than it sounds. The visitor carries
// on through the OIDC flow, the relying party is told they are the attacker,
// and whatever they do next belongs to an account the attacker can read.
func (h *OIDCHandler) checkLoginCSRF(r *http.Request, submitted string) bool {
	stored := h.cookies.get(r, loginCSRFBase)
	if stored == "" || submitted == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(submitted)) == 1
}

// browserEnrolTTL bounds the gap between being refused and finishing. Long
// enough to install an authenticator and transfer a key by hand, short
// enough that a page left open is not a standing way in.
const browserEnrolTTL = 15 * time.Minute

// promptForEnrolment turns an enrol-or-deny refusal into something a person
// can act on without leaving the page.
//
// The API door hands its grant token to the caller and lets them drive
// BeginTOTP/ConfirmTOTP themselves. A browser has no client to do that, so
// this page spends the grant on the visitor's behalf and keeps it
// server-side: what the form carries is an opaque single-use handle, not a
// credential. The grant never reaches the browser at all.
func (h *OIDCHandler) promptForEnrolment(
	w http.ResponseWriter, r *http.Request,
	tenantSlug, realmCode string, d signin.Decision, msg, continueURL string,
) {
	challenge, err := h.granter.Grant(d.Tenant, d.Realm, d.Identity, d.Missing)
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return
	}
	pending, err := h.enrolment.BeginTOTP(r.Context(), challenge.GrantToken)
	if err != nil {
		// A grant is refused if a factor is already enrolled, so somebody who
		// enrolled elsewhere between the password and here lands on the
		// sign-in form rather than on an error: their next attempt asks for
		// the factor they now hold.
		h.renderLogin(w, r, tenantID(d.Tenant), h.resubmit(r, tenantSlug, realmCode,
			"Your account has changed. Please sign in again."))
		return
	}
	handle, err := secret.New(32)
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return
	}
	if _, err := h.onetime.CreateOneTime(r.Context(), d.Tenant.ID, "browser_enrol",
		secret.Hash(handle), jsonx.Must(map[string]string{
			"identity_id":      d.Identity.ID,
			"grant":            challenge.GrantToken,
			"enrollment_token": pending.EnrollmentToken,
			// Set when the member is ALREADY signed in and enrolling by
			// choice inside the grace period. Empty when they were refused,
			// because then there is no session to continue to.
			"continue_url": continueURL,
		}), h.clock.Now().Add(browserEnrolTTL)); err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return
	}
	data := h.resubmit(r, tenantSlug, realmCode, msg)
	data.EnrolToken = handle
	data.EnrolKey = pending.Secret
	data.EnrolURI = pending.ProvisioningURI
	h.renderLogin(w, r, d.Tenant.ID, data)
}

// completeEnrolment finishes the enrolment the page started.
//
// A wrong code costs the whole pending secret, and that is deliberate rather
// than a rough edge: ConfirmTOTP spends the enrolment token BEFORE it checks
// the code, so a stolen grant buys one guess instead of unlimited ones. The
// honest answer to a typo is therefore a new key and a sentence saying so —
// not a retry against a secret that no longer exists.
func (h *OIDCHandler) completeEnrolment(w http.ResponseWriter, r *http.Request, tenantSlug, realmCode, token, code string) {
	restart := func(msg string) {
		h.renderLogin(w, r, "", h.resubmit(r, tenantSlug, realmCode, msg))
	}
	_, payload, err := h.onetime.ConsumeOneTime(r.Context(), "browser_enrol", secret.Hash(token))
	if err != nil {
		restart("That took too long. Please sign in again.")
		return
	}
	var claim struct {
		IdentityID      string `json:"identity_id"`
		Grant           string `json:"grant"`
		EnrollmentToken string `json:"enrollment_token"`
		ContinueURL     string `json:"continue_url"`
	}
	if err := json.Unmarshal(payload, &claim); err != nil || claim.Grant == "" {
		restart("Please sign in again.")
		return
	}
	done, err := h.enrolment.ConfirmTOTP(r.Context(), claim.EnrollmentToken, code, claim.Grant)
	if err != nil {
		h.audit.Emit(r.Context(), auditdomain.AuditEvent{
			TenantID: tenantID(nil), ActorID: claim.IdentityID, ActorKind: "identity",
			Action: "auth.mfa.enrol", Result: "deny", IP: authctx.ClientIP(r.Context()),
			Detail: []byte(`{"surface":"browser","method":"totp"}`),
		})
		restart("That code was not accepted, and the key it was for has been " +
			"discarded. Sign in again to start over with a new one.")
		return
	}
	// Enrolled. The recovery codes exist in readable form exactly once, so
	// they are shown here or never. The sign-in form sits under them: this
	// member now holds the factor their realm demands, so the next attempt is
	// challenged for it — which is also the proof that the enrolment took.
	data := h.resubmit(r, tenantSlug, realmCode, "")
	data.RecoveryCodes = done.RecoveryCodes
	// A member who enrolled from the grace-period warning is already signed
	// in: send them on rather than asking for the password they just used.
	// Punishing somebody for complying early is how a rollout stalls. The
	// refused member has no session, so they get the sign-in form instead.
	data.ContinueURL = claim.ContinueURL
	h.renderLogin(w, r, "", data)
}

// warnEnrolmentDue signs the member in and then says what is coming.
//
// The API returns the deadline and the missing factors on every sign-in
// inside the grace period. The browser had nowhere to put that, because the
// success path redirects to the application in the same response — so this
// page exists to be that place, and it offers the enrolment rather than only
// describing it.
//
// It is shown on every sign-in during the grace period, and it is always
// skippable. A grace period that blocks is not a grace period; one that stays
// quiet is not a warning.
func (h *OIDCHandler) warnEnrolmentDue(w http.ResponseWriter, r *http.Request, tenantSlug, realmCode string, d signin.Decision) {
	if h.openBrowserSession(w, r, d.Tenant, d.Realm, d.Identity, []string{"pwd"}, "password") == "" {
		return
	}
	data := h.resubmit(r, tenantSlug, realmCode, "")
	data.WarnFactors = factorNames(d.Missing)
	data.WarnDeadline = d.Deadline.Format("2 January 2006")
	data.ContinueURL = h.continueURL(r, tenantSlug)
	h.renderLogin(w, r, d.Tenant.ID, data)
}

// enrolFromSession starts enrolment for somebody the warning page just signed
// in. The SSO cookie identifies them, so nothing on the form has to.
func (h *OIDCHandler) enrolFromSession(w http.ResponseWriter, r *http.Request, tenantSlug, realmCode string) {
	raw := h.cookies.get(r, ssoCookieBase)
	if raw == "" {
		h.renderLogin(w, r, "", h.resubmit(r, tenantSlug, realmCode, "Please sign in again."))
		return
	}
	view, err := h.sessions.SessionByCookieHash(r.Context(), secret.Hash(raw))
	if err != nil {
		h.renderLogin(w, r, "", h.resubmit(r, tenantSlug, realmCode, "Please sign in again."))
		return
	}
	tenant, err := h.tenants.TenantBySlug(r.Context(), tenantSlug)
	if err != nil || tenant == nil || tenant.ID != view.TenantID {
		h.renderLogin(w, r, "", h.resubmit(r, tenantSlug, realmCode, "Please sign in again."))
		return
	}
	realm, err := h.realms.RealmByCode(r.Context(), tenant.ID, realmCode)
	if err != nil || realm == nil {
		h.renderLogin(w, r, "", h.resubmit(r, tenantSlug, realmCode, "Please sign in again."))
		return
	}
	identity, err := h.ids.Identity(r.Context(), tenant.ID, view.IdentityID)
	if err != nil || identity == nil {
		h.renderLogin(w, r, "", h.resubmit(r, tenantSlug, realmCode, "Please sign in again."))
		return
	}
	// Which factors are outstanding is the authenticator's question, not this
	// handler's — re-deriving it here is the duplication that put a hole in
	// this file twice.
	missing, err := h.auth.MissingFactors(r.Context(), realm, identity.ID)
	if err != nil {
		apihttp.WriteError(w, r, apperr.ErrInternal.Wrap(err))
		return
	}
	h.promptForEnrolment(w, r, tenantSlug, realmCode, signin.Decision{
		Tenant: tenant, Realm: realm, Identity: identity, Missing: missing,
	}, "", h.continueURL(r, tenantSlug))
}

// continueURL sends the browser back through /v1/authorize, which finds the
// SSO cookie and issues the code without prompting. Re-entering the front
// door keeps PKCE and redirect_uri validation where they already live rather
// than copying them here.
func (h *OIDCHandler) continueURL(r *http.Request, tenantSlug string) string {
	q := url.Values{}
	q.Set("tenant", tenantSlug)
	q.Set("client_id", r.PostFormValue("client_id"))
	q.Set("redirect_uri", r.PostFormValue("redirect_uri"))
	q.Set("response_type", "code")
	q.Set("code_challenge", r.PostFormValue("code_challenge"))
	q.Set("code_challenge_method", r.PostFormValue("code_challenge_method"))
	if v := r.PostFormValue("state"); v != "" {
		q.Set("state", v)
	}
	if v := r.PostFormValue("nonce"); v != "" {
		q.Set("nonce", v)
	}
	return "/v1/authorize?" + q.Encode()
}
