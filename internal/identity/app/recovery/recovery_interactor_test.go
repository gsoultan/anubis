package identityrecovery

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
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
	"github.com/gsoultan/anubis/internal/identity/domain/outbound"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// Stubs embed their port so a method this test does not exercise panics
// naming itself rather than returning a zero value that looks like a result.

type stubTenants struct{ tenancyport.TenantRepository }

func (stubTenants) TenantBySlug(_ context.Context, slug string) (*tenancydomain.TenantRef, error) {
	if slug != "probe" {
		return nil, errors.New("no such tenant")
	}
	return &tenancydomain.TenantRef{ID: "tenant-1", Slug: "probe"}, nil
}

type stubRealms struct{ identityport.RealmRepository }

func probeRealm() *identitydomain.Realm {
	return &identitydomain.Realm{
		ID: "realm-1", Code: "internal", AllowedFactors: []string{"password"},
		PasswordPolicy: identitydomain.PasswordPolicy{MinLength: 12},
	}
}

func (stubRealms) RealmByCode(_ context.Context, _, code string) (*identitydomain.Realm, error) {
	if code != "internal" {
		return nil, errors.New("no such realm")
	}
	return probeRealm(), nil
}

func (stubRealms) RealmByID(context.Context, string) (*identitydomain.Realm, error) {
	return probeRealm(), nil
}

type stubIdentities struct {
	identityport.IdentityRepository
	people  map[string]*identitydomain.Identity // by username and by id
	epochs  int
	lookups int // IdentityForLogin calls: the enumeration-sensitive read
}

func (s *stubIdentities) IdentityForLogin(_ context.Context, _, _, username string) (*identitydomain.Identity, error) {
	s.lookups++
	if p, ok := s.people[username]; ok {
		return p, nil
	}
	return nil, errors.New("not found")
}

func (s *stubIdentities) Identity(_ context.Context, _, id string) (*identitydomain.Identity, error) {
	if p, ok := s.people[id]; ok {
		return p, nil
	}
	return nil, errors.New("not found")
}

func (s *stubIdentities) BumpTokenEpoch(context.Context, string, string) (int, error) {
	s.epochs++
	return s.epochs, nil
}

type stubCreds struct {
	identityport.CredentialRepository
	password     *credential.Credential
	revokedKinds []string
	created      []credential.CredentialInput
}

func (s *stubCreds) PasswordCredential(context.Context, string) (*credential.Credential, error) {
	if s.password == nil {
		return nil, errors.New("none")
	}
	return s.password, nil
}

func (s *stubCreds) RevokeCredentialsOfKind(_ context.Context, _, kind string) (int64, error) {
	s.revokedKinds = append(s.revokedKinds, kind)
	return 1, nil
}

func (s *stubCreds) CreateCredential(_ context.Context, in credential.CredentialInput) (string, error) {
	s.created = append(s.created, in)
	return "cred-new", nil
}

type stubSessions struct {
	authport.SessionRepository
	reasons []string
}

func (s *stubSessions) RevokeAllSessions(_ context.Context, _, _, reason string) ([]authdomain.RevokedSession, error) {
	s.reasons = append(s.reasons, reason)
	return []authdomain.RevokedSession{{ID: "sess-1"}, {ID: "sess-2"}}, nil
}

type stubRefresh struct {
	authport.RefreshRepository
	revokedFor [][]string
}

func (s *stubRefresh) RevokeRefreshBySessions(_ context.Context, ids []string) (int64, error) {
	s.revokedFor = append(s.revokedFor, ids)
	return int64(len(ids)), nil
}

// stubOneTime is the single-use store: consume deletes, and a second consume
// of the same hash is the "not found" the real repository returns.
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
	s.rows[string(hash)] = payload
	s.kinds[string(hash)] = kind
	return "ott-1", nil
}

func (s *stubOneTime) ConsumeOneTime(_ context.Context, kind string, hash []byte) (string, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.rows[string(hash)]
	if !ok || s.kinds[string(hash)] != kind {
		return "", nil, apperr.ErrNotFound
	}
	delete(s.rows, string(hash))
	return "tenant-1", p, nil
}

func (s *stubOneTime) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rows)
}

// stubMail hands each message to a channel so a test can wait for the
// detached delivery instead of sleeping and hoping.
type stubMail struct {
	sent chan outbound.Mail
	fail error
}

func (s *stubMail) Send(_ context.Context, m outbound.Mail) error {
	if s.fail != nil {
		return s.fail
	}
	s.sent <- m
	return nil
}

type stubTx struct{}

func (stubTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }

type stubClock struct{}

func (stubClock) Now() time.Time { return time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC) }

type stubAudit struct {
	mu     sync.Mutex
	events []auditdomain.AuditEvent
}

func (a *stubAudit) Emit(_ context.Context, ev auditdomain.AuditEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, ev)
}

func (a *stubAudit) has(action, result, detail string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, ev := range a.events {
		if ev.Action == action && ev.Result == result && strings.Contains(string(ev.Detail), detail) {
			return true
		}
	}
	return false
}

type probe struct {
	uc       RecoveryUsecase
	ids      *stubIdentities
	creds    *stubCreds
	sessions *stubSessions
	refresh  *stubRefresh
	onetime  *stubOneTime
	mail     *stubMail
	audit    *stubAudit
}

const currentPassword = "the-password-she-knows-2026"

func newProbe(t *testing.T, withMail bool) *probe {
	t.Helper()
	hash, err := kdf.Hash(currentPassword)
	if err != nil {
		t.Fatal(err)
	}
	dewi := &identitydomain.Identity{
		ID: "identity-1", TenantID: "tenant-1", RealmID: "realm-1", Username: "dewi",
		Email: "dewi@example.test", Status: "active",
	}
	gone := &identitydomain.Identity{
		ID: "identity-2", TenantID: "tenant-1", RealmID: "realm-1", Username: "gone",
		Email: "gone@example.test", Status: "disabled", Disabled: true,
	}
	p := &probe{
		ids: &stubIdentities{people: map[string]*identitydomain.Identity{
			"dewi": dewi, "identity-1": dewi, "gone": gone, "identity-2": gone,
		}},
		creds:    &stubCreds{password: &credential.Credential{ID: "cred-1", Kind: "password", Secret: hash}},
		sessions: &stubSessions{},
		refresh:  &stubRefresh{},
		onetime:  &stubOneTime{},
		audit:    &stubAudit{},
	}
	var sender identityport.MailSender
	if withMail {
		p.mail = &stubMail{sent: make(chan outbound.Mail, 4)}
		sender = p.mail
	}
	p.uc = NewRecoveryInteractor(stubTenants{}, stubRealms{}, p.ids, p.creds, p.sessions, p.refresh,
		p.onetime, sender, func(tenant, realm, token string) string {
			return "https://id.example.test/p/" + tenant + "/reset/complete?realm=" + realm + "&token=" + token
		}, stubTx{}, stubClock{}, p.audit, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	return p
}

func (p *probe) request(t *testing.T, username string) error {
	t.Helper()
	return p.uc.RequestPasswordReset(context.Background(), RequestResetInput{
		Tenant: "probe", Realm: "internal", Username: username,
	})
}

func (p *probe) waitMail(t *testing.T) outbound.Mail {
	t.Helper()
	select {
	case m := <-p.mail.sent:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("no mail was delivered")
		return outbound.Mail{}
	}
}

func tokenFrom(t *testing.T, m outbound.Mail) string {
	t.Helper()
	i := strings.Index(m.Text, "token=")
	if i < 0 {
		t.Fatalf("mail carries no token:\n%s", m.Text)
	}
	tok := m.Text[i+len("token="):]
	if j := strings.IndexAny(tok, "\n \r"); j >= 0 {
		tok = tok[:j]
	}
	return tok
}

// The request answers the same for an account that exists and one that does
// not, one that is disabled, and one with no address — nil, every time. The
// difference is in what happens afterwards, which only the operator sees.
func TestARequestAnswersTheSameForEveryUsername(t *testing.T) {
	p := newProbe(t, true)

	for _, u := range []string{"dewi", "nobody", "gone"} {
		if err := p.request(t, u); err != nil {
			t.Fatalf("%s: a reset request leaked an error: %v", u, err)
		}
	}
	m := p.waitMail(t)
	if m.To != "dewi@example.test" || !strings.Contains(m.Text, "token=") {
		t.Fatalf("mail went wrong: %+v", m)
	}
	select {
	case extra := <-p.mail.sent:
		t.Fatalf("a second mail was sent, to %s", extra.To)
	case <-time.After(200 * time.Millisecond):
	}
	if p.onetime.count() != 1 {
		t.Fatalf("expected one live token, have %d", p.onetime.count())
	}
	if !p.audit.has("credential.reset_requested", "allow", "") {
		t.Fatal("the accepted request was not audited")
	}
	if !p.audit.has("credential.reset_requested", "deny", "account_disabled") {
		t.Fatalf("the operator cannot see why the disabled account got no mail: %+v", p.audit.events)
	}
}

// A server with no mail refuses BEFORE any lookup: the same error for a
// username that exists and one that does not, and no token written for
// either — because the refusal must not be a cheaper answer for one of them.
func TestNoMailSenderRefusesBeforeLookingAnyoneUp(t *testing.T) {
	p := newProbe(t, false)

	for _, u := range []string{"dewi", "nobody"} {
		err := p.request(t, u)
		if !errors.Is(err, apperr.ErrMailNotConfigured) {
			t.Fatalf("%s: want mail_not_configured, got %v", u, err)
		}
	}
	if p.ids.lookups != 0 {
		t.Fatalf("the account was looked up %d time(s) before the refusal; the refusal must come first", p.ids.lookups)
	}
	if p.onetime.count() != 0 {
		t.Fatal("a token was minted for a link that can never be sent")
	}
	if len(p.audit.events) != 0 {
		t.Fatalf("a refusal that happens before lookup audited something: %+v", p.audit.events)
	}
}

// The whole flow: the emailed token sets the password exactly once, and the
// old password's sessions do not survive it.
func TestTheLinkSetsThePasswordOnceAndSignsOutEverywhere(t *testing.T) {
	p := newProbe(t, true)
	if err := p.request(t, "dewi"); err != nil {
		t.Fatal(err)
	}
	token := tokenFrom(t, p.waitMail(t))

	const next = "a-brand-new-password-2026"
	if err := p.uc.CompletePasswordReset(context.Background(), token, next); err != nil {
		t.Fatalf("completing with the emailed token failed: %v", err)
	}
	if len(p.creds.revokedKinds) != 1 || p.creds.revokedKinds[0] != "password" {
		t.Fatalf("old password not retired: %v", p.creds.revokedKinds)
	}
	if len(p.creds.created) != 1 {
		t.Fatalf("expected one new credential, got %d", len(p.creds.created))
	}
	if ok, rehash, err := kdf.Verify(next, p.creds.created[0].Secret); err != nil || !ok || rehash {
		t.Fatal("the stored hash does not verify the new password")
	}
	if len(p.sessions.reasons) != 1 || p.sessions.reasons[0] != "password_reset" {
		t.Fatalf("sessions not revoked with the reset reason: %v", p.sessions.reasons)
	}
	if len(p.refresh.revokedFor) != 1 || len(p.refresh.revokedFor[0]) != 2 {
		t.Fatalf("refresh families of the revoked sessions survived: %v", p.refresh.revokedFor)
	}
	if p.ids.epochs != 1 {
		t.Fatalf("token epoch not bumped: %d", p.ids.epochs)
	}
	if !p.audit.has("credential.password_reset", "allow", "") {
		t.Fatal("the reset was not audited")
	}

	// Single use: the same link again is dead.
	err := p.uc.CompletePasswordReset(context.Background(), token, "yet-another-password-2026")
	if !errors.Is(err, apperr.ErrTokenInvalid) {
		t.Fatalf("a used link was accepted again: %v", err)
	}
	if len(p.creds.created) != 1 {
		t.Fatal("the second use wrote a credential")
	}
}

// A password the realm refuses burns the link. Pinned so the page can say
// so honestly rather than offering a retry that cannot work.
func TestAPasswordTheRealmRefusesBurnsTheLink(t *testing.T) {
	p := newProbe(t, true)
	if err := p.request(t, "dewi"); err != nil {
		t.Fatal(err)
	}
	token := tokenFrom(t, p.waitMail(t))

	err := p.uc.CompletePasswordReset(context.Background(), token, "short")
	if apperr.AsError(err).Code != "password_policy" {
		t.Fatalf("want password_policy, got %v", err)
	}
	if len(p.creds.created)+len(p.creds.revokedKinds)+len(p.sessions.reasons) != 0 {
		t.Fatal("a refused password wrote something")
	}
	err = p.uc.CompletePasswordReset(context.Background(), token, "long-enough-this-time-2026")
	if !errors.Is(err, apperr.ErrTokenInvalid) {
		t.Fatalf("the burned link still worked: %v", err)
	}
}

// Delivery failing after the answer is not silent: the operator gets a log
// line and an audit event they can alert on.
func TestADeliveryFailureReachesTheOperator(t *testing.T) {
	p := newProbe(t, true)
	p.mail.fail = errors.New("relay refused")
	if err := p.request(t, "dewi"); err != nil {
		t.Fatalf("the person was told about a relay failure: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !p.audit.has("credential.reset_mail", "deny", "delivery_failed") {
		if time.Now().After(deadline) {
			t.Fatalf("delivery failure never audited: %+v", p.audit.events)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func signedIn(platform bool) context.Context {
	return authctx.With(context.Background(), &authctx.Principal{
		IdentityID: "identity-1", TenantID: "tenant-1", SessionID: "sess-1", Platform: platform,
	})
}

// A session is not enough to change the password; the current password is.
func TestChangePasswordNeedsTheCurrentPassword(t *testing.T) {
	p := newProbe(t, true)

	err := p.uc.ChangePassword(signedIn(false), "not-the-password-at-all", "a-brand-new-password-2026")
	if !errors.Is(err, apperr.ErrInvalidCredentials) {
		t.Fatalf("wrong current password: want invalid_credentials, got %v", err)
	}
	if len(p.creds.created)+len(p.sessions.reasons) != 0 {
		t.Fatal("a wrong current password wrote something")
	}
	if !p.audit.has("credential.password_changed", "deny", "invalid_credentials") {
		t.Fatal("the refused change was not audited")
	}

	if err := p.uc.ChangePassword(signedIn(false), currentPassword, currentPassword); apperr.AsError(err).Code != "password_policy" {
		t.Fatalf("the same password again: want password_policy, got %v", err)
	}

	if err := p.uc.ChangePassword(signedIn(false), currentPassword, "a-brand-new-password-2026"); err != nil {
		t.Fatalf("a correct change failed: %v", err)
	}
	if len(p.creds.created) != 1 || len(p.sessions.reasons) != 1 || p.sessions.reasons[0] != "password_changed" {
		t.Fatalf("change did not rekey and sign out: created %d reasons %v", len(p.creds.created), p.sessions.reasons)
	}
	if p.ids.epochs != 1 {
		t.Fatal("token epoch not bumped on change")
	}
}

// Platform operators and services are not tenant people; the call refuses
// them before touching a credential.
func TestChangePasswordIsForTenantPeopleOnly(t *testing.T) {
	p := newProbe(t, true)
	for name, ctx := range map[string]context.Context{
		"platform operator": signedIn(true),
		"service":           authctx.With(context.Background(), &authctx.Principal{IdentityID: "svc", TenantID: "tenant-1", Service: true}),
		"nobody":            context.Background(),
	} {
		err := p.uc.ChangePassword(ctx, currentPassword, "a-brand-new-password-2026")
		if !errors.Is(err, apperr.ErrUnauthenticated) {
			t.Errorf("%s: want unauthenticated, got %v", name, err)
		}
	}
	if len(p.creds.created) != 0 {
		t.Fatal("a refused caller wrote a credential")
	}
}
