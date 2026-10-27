package emaillink

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/outbound"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

type stubTenants struct{ tenancyport.TenantRepository }

func (stubTenants) TenantBySlug(_ context.Context, slug string) (*tenancydomain.TenantRef, error) {
	if slug != "careers" {
		return nil, errors.New("no")
	}
	return &tenancydomain.TenantRef{ID: "t1", Slug: "careers"}, nil
}

func (stubTenants) TenantByID(context.Context, string) (*tenancydomain.TenantRef, error) {
	return &tenancydomain.TenantRef{ID: "t1", Slug: "careers"}, nil
}

type stubRealms struct {
	identityport.RealmRepository
	factors []string
}

func (s stubRealms) RealmByCode(context.Context, string, string) (*identitydomain.Realm, error) {
	return &identitydomain.Realm{ID: "r-public", Code: "public", AllowedFactors: s.factors}, nil
}

func (s stubRealms) RealmByID(context.Context, string) (*identitydomain.Realm, error) {
	return &identitydomain.Realm{ID: "r-public", Code: "public", AllowedFactors: s.factors}, nil
}

type stubIdentities struct {
	identityport.IdentityRepository
	people map[string]*identitydomain.Identity
}

func (s stubIdentities) IdentityForLogin(_ context.Context, _, _, username string) (*identitydomain.Identity, error) {
	if p, ok := s.people[username]; ok {
		return p, nil
	}
	return nil, errors.New("not found")
}

func (s stubIdentities) Identity(_ context.Context, _, id string) (*identitydomain.Identity, error) {
	if p, ok := s.people[id]; ok {
		return p, nil
	}
	return nil, errors.New("not found")
}

type stubOneTime struct {
	authport.OneTimeRepository
	mu    sync.Mutex
	rows  map[string][]byte
	kinds map[string]string
}

func (s *stubOneTime) CreateOneTime(_ context.Context, _ string, kind string, hash []byte, payload []byte, _ time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rows == nil {
		s.rows, s.kinds = map[string][]byte{}, map[string]string{}
	}
	s.rows[string(hash)], s.kinds[string(hash)] = payload, kind
	return "ott", nil
}

func (s *stubOneTime) ConsumeOneTime(_ context.Context, kind string, hash []byte) (string, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.rows[string(hash)]
	if !ok || s.kinds[string(hash)] != kind {
		return "", nil, apperr.ErrNotFound
	}
	delete(s.rows, string(hash))
	return "t1", p, nil
}

type stubMail struct{ sent chan outbound.Mail }

func (s *stubMail) Send(_ context.Context, m outbound.Mail) error { s.sent <- m; return nil }

type stubClock struct{}

func (stubClock) Now() time.Time { return time.Unix(1_700_000_000, 0) }

type stubAudit struct {
	mu     sync.Mutex
	events []auditdomain.AuditEvent
}

func (a *stubAudit) Emit(_ context.Context, ev auditdomain.AuditEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, ev)
}

func probe(factors []string, withMail bool) (Usecase, *stubMail, *stubOneTime, *stubAudit) {
	ayu := &identitydomain.Identity{ID: "i-ayu", TenantID: "t1", RealmID: "r-public", Username: "ayu@mail.example", Email: "ayu@mail.example", Status: "active"}
	gone := &identitydomain.Identity{ID: "i-gone", TenantID: "t1", RealmID: "r-public", Username: "gone@mail.example", Email: "gone@mail.example", Status: "disabled", Disabled: true}
	mail := &stubMail{sent: make(chan outbound.Mail, 4)}
	var sender identityport.MailSender
	if withMail {
		sender = mail
	}
	ott := &stubOneTime{}
	audit := &stubAudit{}
	uc := New(stubTenants{}, stubRealms{factors: factors},
		stubIdentities{people: map[string]*identitydomain.Identity{"ayu@mail.example": ayu, "i-ayu": ayu, "gone@mail.example": gone, "i-gone": gone}},
		ott, sender, func(tok string) string { return "https://id.example.test/v1/login/email?token=" + tok },
		stubClock{}, audit, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	return uc, mail, ott, audit
}

func request(uc Usecase, username string) error {
	return uc.Request(context.Background(), RequestInput{Tenant: "careers", Realm: "public", Username: username,
		Flow: map[string]string{"client_id": "careers-web", "redirect_uri": "https://careers.example.test/cb", "state": "s1"}})
}

func tokenFrom(t *testing.T, m outbound.Mail) string {
	t.Helper()
	i := strings.Index(m.Text, "token=")
	if i < 0 {
		t.Fatalf("no token in mail:\n%s", m.Text)
	}
	tok := m.Text[i+6:]
	if j := strings.IndexAny(tok, "\n "); j >= 0 {
		tok = tok[:j]
	}
	return tok
}

// The door: a link arrives for an account that exists and may sign in, the
// link signs that person in ONCE, and the flow they started comes back out.
func TestTheLinkSignsInOnceAndReturnsTheFlow(t *testing.T) {
	uc, mail, _, _ := probe([]string{Factor}, true)
	for _, u := range []string{"ayu@mail.example", "nobody@mail.example", "gone@mail.example"} {
		if err := request(uc, u); err != nil {
			t.Fatalf("%s: a request leaked an error: %v", u, err)
		}
	}
	var m outbound.Mail
	select {
	case m = <-mail.sent:
	case <-time.After(5 * time.Second):
		t.Fatal("no mail")
	}
	if m.To != "ayu@mail.example" {
		t.Fatalf("mail to %s", m.To)
	}
	select {
	case extra := <-mail.sent:
		t.Fatalf("a second mail went to %s", extra.To)
	case <-time.After(200 * time.Millisecond):
	}

	done, err := uc.Complete(context.Background(), tokenFrom(t, m))
	if err != nil || done.Identity.ID != "i-ayu" || done.Flow["client_id"] != "careers-web" || done.Tenant.Slug != "careers" {
		t.Fatalf("complete: %+v %v", done, err)
	}
	if _, err := uc.Complete(context.Background(), tokenFrom(t, m)); !errors.Is(err, apperr.ErrTokenInvalid) {
		t.Fatalf("the link worked twice: %v", err)
	}
}

// The door is a realm setting: a realm that does not allow email_link sends
// nothing, and a link requested before the realm turned it off stops
// working when it does.
func TestTheDoorIsARealmSetting(t *testing.T) {
	uc, mail, _, _ := probe([]string{"password"}, true)
	if err := request(uc, "ayu@mail.example"); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-mail.sent:
		t.Fatalf("a realm without the factor mailed %s", m.To)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestNoMailRefusesBeforeAnyLookup(t *testing.T) {
	uc, _, ott, audit := probe([]string{Factor}, false)
	for _, u := range []string{"ayu@mail.example", "nobody@mail.example"} {
		if err := request(uc, u); !errors.Is(err, apperr.ErrMailNotConfigured) {
			t.Fatalf("%s: want mail_not_configured, got %v", u, err)
		}
	}
	if len(ott.rows) != 0 || len(audit.events) != 0 {
		t.Fatal("a refusal before lookup wrote a token or an audit event")
	}
}
