package authhttp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/auth/app/emaillink"
	"github.com/gsoultan/anubis/internal/auth/app/signin"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

// The e-mail door is a sign-in, so it answers to the same factor policy as a
// password: the factor belongs to the identity, not to the door.

type linkUsecase struct {
	done     *emaillink.Completed
	consumed int
}

func (u *linkUsecase) Request(context.Context, emaillink.RequestInput) error { return nil }

func (u *linkUsecase) Complete(_ context.Context, token string) (*emaillink.Completed, error) {
	u.consumed++
	if token != "the-emailed-token" || u.consumed > 1 {
		return nil, errNoPage
	}
	return u.done, nil
}

type linkCreds struct {
	identityport.CredentialRepository
	kinds []string
}

func (c linkCreds) ActiveCredentialKinds(context.Context, string) ([]string, error) {
	return c.kinds, nil
}

type linkOneTime struct {
	authport.OneTimeRepository
	created map[string][]byte
}

func (o *linkOneTime) CreateOneTime(_ context.Context, _, kind string, _ []byte, payload []byte, _ time.Time) (string, error) {
	o.created[kind] = payload
	return "ot-1", nil
}

type linkSessions struct {
	authport.SessionRepository
	opened []authdomain.SessionInput
}

func (s *linkSessions) CreateSession(_ context.Context, in authdomain.SessionInput) (*authdomain.Session, error) {
	s.opened = append(s.opened, in)
	return &authdomain.Session{ID: "sess-1"}, nil
}

func (s *linkSessions) SetSessionCookieHash(context.Context, string, []byte) error { return nil }

type linkAudit struct{}

func (linkAudit) Emit(context.Context, auditdomain.AuditEvent) {}

type linkClock struct{}

func (linkClock) Now() time.Time { return time.Unix(1_700_000_000, 0) }

type linkProbe struct {
	h        *OIDCHandler
	link     *linkUsecase
	sessions *linkSessions
	onetime  *linkOneTime
}

func newLinkProbe(enrolled ...string) *linkProbe {
	tenant := &tenancydomain.TenantRef{ID: "tenant-1", Slug: "acme"}
	realm := &identitydomain.Realm{ID: "realm-1", Code: "applicants", SessionTTL: time.Hour,
		AllowedFactors: []string{"email_link", "totp"}}
	p := &linkProbe{
		link: &linkUsecase{done: &emaillink.Completed{
			Tenant: tenant, Realm: realm,
			Identity: &identitydomain.Identity{ID: "ida", TenantID: "tenant-1", Status: "active"},
			Flow:     map[string]string{},
		}},
		sessions: &linkSessions{},
		onetime:  &linkOneTime{created: map[string][]byte{}},
	}
	p.h = &OIDCHandler{
		tenants:   &stubTenants{id: "tenant-1", slug: "acme"},
		realms:    &stubRealms{},
		pages:     &stubPages{},
		emailLink: p.link,
		auth:      signin.NewPasswordAuthenticator(nil, nil, nil, linkCreds{kinds: enrolled}, linkClock{}, linkAudit{}),
		sessions:  p.sessions,
		onetime:   p.onetime,
		renderer:  NewPageRenderer(),
		clock:     linkClock{},
		audit:     linkAudit{},
		logger:    slog.Default(),
	}
	return p
}

var csrfField = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

// open is the person's click: the GET the mail's link makes, then the button
// on the page it shows.
func (p *linkProbe) open(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	get := httptest.NewRecorder()
	p.h.EmailLinkOpen(get, httptest.NewRequest("GET", "/v1/login/email?token=the-emailed-token&tenant=acme", nil))
	m := csrfField.FindStringSubmatch(get.Body.String())
	if m == nil {
		t.Fatalf("the link's page offers nothing to confirm with:\n%s", get.Body.String())
	}
	form := url.Values{"token": {"the-emailed-token"}, "csrf": {m[1]}}
	post := httptest.NewRequest("POST", "/v1/login/email/open", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range get.Result().Cookies() {
		post.AddCookie(c)
	}
	w := httptest.NewRecorder()
	p.h.EmailLinkConfirm(w, post)
	return w
}

// Opening the link signed the person in on the GET, so a mail client or
// security scanner that fetches every link spent it, and was signed in as
// them, before they saw it. The GET now only shows a button.
func TestOpeningTheLinkDoesNotSpendIt(t *testing.T) {
	p := newLinkProbe()
	w := httptest.NewRecorder()
	p.h.EmailLinkOpen(w, httptest.NewRequest("GET", "/v1/login/email?token=the-emailed-token&tenant=acme", nil))

	if p.link.consumed != 0 || len(p.sessions.opened) != 0 {
		t.Fatalf("a GET consumed the link (%d) and opened %d sessions", p.link.consumed, len(p.sessions.opened))
	}
	if !strings.Contains(w.Body.String(), `value="the-emailed-token"`) {
		t.Error("the page does not carry the token on to the confirming POST")
	}
}

// The link opened a session even when the person had enrolled an
// authenticator: the door went straight to openBrowserSession and never asked
// the authenticator that applies the factor policy. Now it asks, and an
// enrolled factor is asked for before any session exists.
func TestAnEnrolledFactorIsAskedForAfterTheLink(t *testing.T) {
	p := newLinkProbe("totp")
	w := p.open(t)

	if len(p.sessions.opened) != 0 {
		t.Fatalf("a session was opened with an enrolled authenticator never asked for: %+v", p.sessions.opened)
	}
	if !strings.Contains(w.Body.String(), `name="mfa_token"`) {
		t.Fatalf("the page does not ask for the code:\n%s", w.Body.String())
	}
	var claim map[string]string
	if err := json.Unmarshal(p.onetime.created["browser_mfa"], &claim); err != nil || claim["identity_id"] != "ida" {
		t.Fatalf("second-factor token payload = %s", p.onetime.created["browser_mfa"])
	}
	// The code step must not record a password nobody typed.
	if claim["primary"] != emaillink.Factor {
		t.Errorf("second-factor token says the first factor was %q, want %q", claim["primary"], emaillink.Factor)
	}
}

func TestWithNoFactorTheLinkSignsIn(t *testing.T) {
	p := newLinkProbe()
	w := p.open(t)

	if len(p.sessions.opened) != 1 || !slices.Equal(p.sessions.opened[0].AMR, []string{emaillink.Factor}) {
		t.Fatalf("sessions = %+v, want one with amr [email_link]", p.sessions.opened)
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want a redirect to the sign-in page", w.Code)
	}
	if again := p.open(t); len(p.sessions.opened) != 1 || again.Code != http.StatusBadRequest {
		t.Error("a spent link signed in a second time")
	}
}

// The page a stale form lands on still works: the link was not spent.
func TestAForgedConfirmSpendsNothing(t *testing.T) {
	p := newLinkProbe()
	form := url.Values{"token": {"the-emailed-token"}, "csrf": {"not-ours"}}
	r := httptest.NewRequest("POST", "/v1/login/email/open", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	p.h.EmailLinkConfirm(httptest.NewRecorder(), r)
	if p.link.consumed != 0 || len(p.sessions.opened) != 0 {
		t.Fatal("a cross-site POST consumed the link")
	}
}
