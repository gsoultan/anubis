package identityrecovery

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
	"github.com/gsoultan/anubis/internal/identity/domain/outbound"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/txm"
	"github.com/gsoultan/anubis/internal/shared/validate"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// ResetTTL is how long an emailed link works. Long enough to find the mail,
// short enough that a link left in an inbox is not a standing credential.
const ResetTTL = 30 * time.Minute

// sendTimeout bounds one delivery attempt, which runs after the request has
// already been answered (see RequestPasswordReset).
const sendTimeout = 30 * time.Second

// LinkBuilder turns a fresh token into the URL the mail carries. The
// composition root supplies it because only there is the issuer known.
type LinkBuilder func(tenantSlug, realmCode, token string) string

type recoveryInteractor struct {
	tenants  tenancyport.TenantRepository
	realms   identityport.RealmRepository
	ids      identityport.IdentityRepository
	creds    identityport.CredentialRepository
	sessions authport.SessionRepository
	refresh  authport.RefreshRepository
	onetime  authport.OneTimeRepository
	mail     identityport.MailSender
	link     LinkBuilder
	tx       txm.TxManager
	clock    clock.Clock
	audit    auditport.Auditor
	logger   *slog.Logger
}

// NewRecoveryInteractor wires the usecase. mail may be nil: the installation
// then cannot send a reset link and RequestPasswordReset says so.
func NewRecoveryInteractor(
	tenants tenancyport.TenantRepository,
	realms identityport.RealmRepository,
	ids identityport.IdentityRepository,
	creds identityport.CredentialRepository,
	sessions authport.SessionRepository,
	refresh authport.RefreshRepository,
	onetime authport.OneTimeRepository,
	mail identityport.MailSender,
	link LinkBuilder,
	tx txm.TxManager,
	clk clock.Clock,
	audit auditport.Auditor,
	logger *slog.Logger,
) RecoveryUsecase {
	return &recoveryInteractor{
		tenants: tenants, realms: realms, ids: ids, creds: creds,
		sessions: sessions, refresh: refresh, onetime: onetime,
		mail: mail, link: link, tx: tx, clock: clk, audit: audit, logger: logger,
	}
}

func (u *recoveryInteractor) RequestPasswordReset(ctx context.Context, in RequestResetInput) error {
	// Refused BEFORE any lookup, so it is the same refusal for every
	// username: a server that cannot send mail must not say so only for
	// accounts that exist.
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
	if err != nil || realm == nil || !realm.AllowsFactor("password") {
		return nil
	}
	identity, err := u.ids.IdentityForLogin(ctx, tenant.ID, realm.ID, in.Username)
	if err != nil || identity == nil {
		return nil
	}
	// Every refusal from here on is silent to the caller and loud to the
	// audit log — the person is told "check your email" either way, and
	// the operator can see why nothing arrived.
	if aerr := identity.CanAuthenticate(); aerr != nil {
		u.emit(ctx, tenant.ID, identity.ID, "credential.reset_requested", "deny", apperr.AsError(aerr).Code)
		return nil
	}
	if identity.Email == "" {
		u.emit(ctx, tenant.ID, identity.ID, "credential.reset_requested", "deny", "no_email")
		return nil
	}

	token, err := secret.New(32)
	if err != nil {
		return apperr.ErrInternal.Wrap(err)
	}
	if _, err := u.onetime.CreateOneTime(ctx, tenant.ID, "password_reset", secret.Hash(token),
		jsonx.Must(map[string]string{"identity_id": identity.ID, "tenant_id": tenant.ID}),
		u.clock.Now().Add(ResetTTL)); err != nil {
		return apperr.ErrInternal.Wrap(err)
	}
	u.emit(ctx, tenant.ID, identity.ID, "credential.reset_requested", "allow", "")

	// Delivery runs after the answer, on its own clock. Not for speed: an
	// SMTP conversation takes hundreds of milliseconds and a lookup that
	// finds nobody takes none, and that gap would say which usernames
	// exist. The answer is the same for everyone and takes the same time.
	m := outbound.Mail{
		To:      identity.Email,
		Subject: "Reset your password",
		Text:    resetMailText(identity.Username, u.link(tenant.Slug, realm.Code, token)),
	}
	go u.deliver(tenant.ID, identity.ID, m)
	return nil
}

// deliver sends one message detached from the request that asked for it.
// A failure cannot reach the person, so it reaches the operator twice: the
// log, and an audit event they can alert on.
func (u *recoveryInteractor) deliver(tenantID, identityID string, m outbound.Mail) {
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	if err := u.mail.Send(ctx, m); err != nil {
		u.logger.Error("password reset mail not delivered", "tenant", tenantID, "identity", identityID, "error", err)
		u.emit(ctx, tenantID, identityID, "credential.reset_mail", "deny", "delivery_failed")
		return
	}
	u.emit(ctx, tenantID, identityID, "credential.reset_mail", "allow", "")
}

func (u *recoveryInteractor) CompletePasswordReset(ctx context.Context, token, password string) error {
	if token == "" || len(token) > 128 {
		return apperr.ErrTokenInvalid
	}
	tenantID, payload, err := u.onetime.ConsumeOneTime(ctx, "password_reset", secret.Hash(token))
	if err != nil {
		return apperr.ErrTokenInvalid
	}
	var p struct {
		IdentityID string `json:"identity_id"`
	}
	if json.Unmarshal(payload, &p) != nil || p.IdentityID == "" {
		return apperr.ErrTokenInvalid
	}
	identity, err := u.ids.Identity(ctx, tenantID, p.IdentityID)
	if err != nil || identity == nil {
		return apperr.ErrTokenInvalid
	}
	if aerr := identity.CanAuthenticate(); aerr != nil {
		return aerr
	}
	if err := u.rekey(ctx, identity, password, "password_reset"); err != nil {
		return err
	}
	u.emit(ctx, tenantID, identity.ID, "credential.password_reset", "allow", "")
	return nil
}

func (u *recoveryInteractor) ChangePassword(ctx context.Context, current, next string) error {
	p, ok := authctx.From(ctx)
	// A platform operator has ChangePlatformPassword; a service has no
	// password. Neither is a tenant person, and this call is about one.
	if !ok || p.IdentityID == "" || p.Platform || p.Service {
		return apperr.ErrUnauthenticated
	}
	identity, err := u.ids.Identity(ctx, p.TenantID, p.IdentityID)
	if err != nil || identity == nil {
		return apperr.ErrUnauthenticated
	}
	if aerr := identity.CanAuthenticate(); aerr != nil {
		return aerr
	}
	hash := kdf.Dummy()
	cred, err := u.creds.PasswordCredential(ctx, identity.ID)
	if err == nil && cred != nil && cred.Secret != "" {
		hash = cred.Secret
	}
	ok, _, kerr := kdf.Verify(current, hash) // kdf-rehash-exempt: replaced below
	if kerr != nil || cred == nil || !ok {
		u.emit(ctx, p.TenantID, identity.ID, "credential.password_changed", "deny", "invalid_credentials")
		return apperr.ErrInvalidCredentials
	}
	if next == current {
		return apperr.ErrPasswordPolicy.With("reuse", "the new password must differ from the current one")
	}
	if err := u.rekey(ctx, identity, next, "password_changed"); err != nil {
		return err
	}
	u.emit(ctx, p.TenantID, identity.ID, "credential.password_changed", "allow", "")
	return nil
}

// rekey is the one write both paths share: the realm's policy, a fresh
// hash, every session and refresh family revoked, and the epoch bumped so
// that no outstanding access token survives the password it was issued
// under. Reason names which path did it, on every revoked session.
func (u *recoveryInteractor) rekey(ctx context.Context, identity *identitydomain.Identity, password, reason string) error {
	realm, err := u.realms.RealmByID(ctx, identity.RealmID)
	if err != nil || realm == nil {
		return apperr.ErrInternal.Wrap(err)
	}
	if err := realm.PasswordPolicy.Check(password); err != nil {
		return err
	}
	hash, err := kdf.Hash(password)
	if err != nil {
		return apperr.ErrInternal.Wrap(err)
	}
	return u.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := u.creds.RevokeCredentialsOfKind(ctx, identity.ID, "password"); err != nil {
			return apperr.ErrInternal.Wrap(err)
		}
		if _, err := u.creds.CreateCredential(ctx, credential.CredentialInput{
			IdentityID: identity.ID, TenantID: identity.TenantID, Kind: "password", Secret: hash,
		}); err != nil {
			return apperr.ErrInternal.Wrap(err)
		}
		revoked, err := u.sessions.RevokeAllSessions(ctx, identity.TenantID, identity.ID, reason)
		if err != nil {
			return apperr.ErrInternal.Wrap(err)
		}
		sids := make([]string, len(revoked))
		for i, s := range revoked {
			sids[i] = s.ID
		}
		if len(sids) > 0 {
			if _, err := u.refresh.RevokeRefreshBySessions(ctx, sids); err != nil {
				return apperr.ErrInternal.Wrap(err)
			}
		}
		if _, err := u.ids.BumpTokenEpoch(ctx, identity.TenantID, identity.ID); err != nil {
			return apperr.ErrInternal.Wrap(err)
		}
		return nil
	})
}

func (u *recoveryInteractor) emit(ctx context.Context, tenantID, identityID, action, result, detail string) {
	ev := auditdomain.AuditEvent{
		TenantID: tenantID, ActorID: identityID, ActorKind: "identity",
		TargetID: identityID, Action: action, Result: result,
		IP: authctx.ClientIP(ctx),
	}
	if detail != "" {
		ev.Detail = jsonx.Must(map[string]string{"detail": detail})
	}
	u.audit.Emit(ctx, ev)
}

// resetMailText is deliberately plain: no markup, no tracking, nothing but
// what the person needs to decide whether this was them.
func resetMailText(username, link string) string {
	return "Someone asked to reset the password for " + username + ".\n\n" +
		"To choose a new password, open this link within " + ResetTTL.String() + ":\n\n" +
		"  " + link + "\n\n" +
		"The link works once. If you did not ask for this, you can ignore this\n" +
		"email — your password has not changed.\n"
}
