package emaillink

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	"github.com/gsoultan/anubis/internal/identity/domain/outbound"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/validate"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// Factor is the realm factor code that turns this door on.
const Factor = "email_link"

// LinkTTL: long enough to find the mail, short enough that a link left in
// an inbox is not a standing credential.
const LinkTTL = 15 * time.Minute

const sendTimeout = 30 * time.Second

// LinkBuilder turns a token into the URL the mail carries.
type LinkBuilder func(token string) string

// payload is what the one-time token remembers between the two halves.
type payload struct {
	IdentityID string            `json:"identity_id"`
	TenantID   string            `json:"tenant_id"`
	RealmID    string            `json:"realm_id"`
	Flow       map[string]string `json:"flow,omitempty"`
}

type interactor struct {
	tenants tenancyport.TenantRepository
	realms  identityport.RealmRepository
	ids     identityport.IdentityRepository
	onetime authport.OneTimeRepository
	mail    identityport.MailSender
	link    LinkBuilder
	clock   clock.Clock
	audit   auditport.Auditor
	logger  *slog.Logger
}

func New(
	tenants tenancyport.TenantRepository,
	realms identityport.RealmRepository,
	ids identityport.IdentityRepository,
	onetime authport.OneTimeRepository,
	mail identityport.MailSender,
	link LinkBuilder,
	clk clock.Clock,
	audit auditport.Auditor,
	logger *slog.Logger,
) Usecase {
	return &interactor{
		tenants: tenants, realms: realms, ids: ids, onetime: onetime,
		mail: mail, link: link, clock: clk, audit: audit, logger: logger,
	}
}

func (u *interactor) Request(ctx context.Context, in RequestInput) error {
	// Refused BEFORE any lookup, so it is the same refusal for every
	// username (see the password reset, which shares the rule).
	if u.mail == nil {
		return apperr.ErrMailNotConfigured
	}
	if in.Realm == "" {
		in.Realm = "internal"
	}
	if !validate.ValidSlug(in.Tenant) || !validate.ValidUsername(in.Username) {
		return nil
	}
	tenant, err := u.tenants.TenantBySlug(ctx, in.Tenant)
	if err != nil || tenant == nil {
		return nil
	}
	realm, err := u.realms.RealmByCode(ctx, tenant.ID, in.Realm)
	if err != nil || realm == nil || !realm.AllowsFactor(Factor) {
		return nil
	}
	identity, err := u.ids.IdentityForLogin(ctx, tenant.ID, realm.ID, in.Username)
	if err != nil || identity == nil {
		return nil
	}
	if aerr := identity.CanAuthenticate(); aerr != nil {
		u.emit(ctx, tenant.ID, identity.ID, "deny", apperr.AsError(aerr).Code, "")
		return nil
	}
	if identity.Email == "" {
		u.emit(ctx, tenant.ID, identity.ID, "deny", "no_email", "")
		return nil
	}
	token, err := secret.New(32)
	if err != nil {
		return apperr.ErrInternal.Wrap(err)
	}
	if _, err := u.onetime.CreateOneTime(ctx, tenant.ID, "email_login", secret.Hash(token),
		jsonx.Must(payload{IdentityID: identity.ID, TenantID: tenant.ID, RealmID: realm.ID, Flow: in.Flow}),
		u.clock.Now().Add(LinkTTL)); err != nil {
		return apperr.ErrInternal.Wrap(err)
	}
	u.emit(ctx, tenant.ID, identity.ID, "allow", "link_requested", "")

	// Delivery after the answer, on its own clock — SMTP latency must not
	// say which usernames exist.
	m := outbound.Mail{
		To:      identity.Email,
		Subject: "Your sign-in link",
		Text: "Open this link within " + LinkTTL.String() + " to sign in as " + identity.Username + ":\n\n" +
			"  " + u.link(token) + "\n\n" +
			"The link works once. If you did not ask for it, ignore this email; nobody\n" +
			"can use it without opening it.\n",
	}
	go u.deliver(tenant.ID, identity.ID, m)
	return nil
}

func (u *interactor) deliver(tenantID, identityID string, m outbound.Mail) {
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	if err := u.mail.Send(ctx, m); err != nil {
		u.logger.Error("sign-in link not delivered", "tenant", tenantID, "identity", identityID, "error", err)
		u.emit(ctx, tenantID, identityID, "deny", "delivery_failed", "")
		return
	}
	u.emit(ctx, tenantID, identityID, "allow", "link_sent", "")
}

func (u *interactor) Complete(ctx context.Context, token string) (*Completed, error) {
	if token == "" || len(token) > 128 {
		return nil, apperr.ErrTokenInvalid
	}
	tenantID, raw, err := u.onetime.ConsumeOneTime(ctx, "email_login", secret.Hash(token))
	if err != nil {
		return nil, apperr.ErrTokenInvalid
	}
	var p payload
	if json.Unmarshal(raw, &p) != nil || p.IdentityID == "" || p.TenantID != tenantID {
		return nil, apperr.ErrTokenInvalid
	}
	identity, err := u.ids.Identity(ctx, tenantID, p.IdentityID)
	if err != nil || identity == nil {
		return nil, apperr.ErrTokenInvalid
	}
	if aerr := identity.CanAuthenticate(); aerr != nil {
		u.emit(ctx, tenantID, identity.ID, "deny", apperr.AsError(aerr).Code, "")
		return nil, apperr.ErrTokenInvalid
	}
	realm, err := u.realms.RealmByID(ctx, p.RealmID)
	if err != nil || realm == nil || !realm.AllowsFactor(Factor) {
		// The realm stopped allowing the door between request and click.
		u.emit(ctx, tenantID, identity.ID, "deny", "factor_not_allowed", "")
		return nil, apperr.ErrTokenInvalid
	}
	tenant, err := u.tenants.TenantByID(ctx, tenantID)
	if err != nil || tenant == nil {
		return nil, apperr.ErrTokenInvalid
	}
	return &Completed{Tenant: tenant, Realm: realm, Identity: identity, Flow: p.Flow}, nil
}

func (u *interactor) emit(ctx context.Context, tenantID, identityID, result, detail, sessionID string) {
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: tenantID, ActorID: identityID, ActorKind: "identity",
		SessionID: sessionID, Action: "auth.email_link", Result: result,
		IP: authctx.ClientIP(ctx), Detail: jsonx.Must(map[string]string{"detail": detail}),
	})
}
