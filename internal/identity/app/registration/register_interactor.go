package registration

import (
	"context"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
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

const emailVerifyTTL = 24 * time.Hour

// registerInteractor implements RegisterUsecase.
type registerInteractor struct {
	tenants  tenancyport.TenantRepository
	realms   identityport.RealmRepository
	ids      identityport.IdentityRepository
	creds    identityport.CredentialRepository
	consents identityport.ConsentRepository
	onetime  authport.OneTimeRepository
	tx       txm.TxManager
	clock    clock.Clock
	audit    auditport.Auditor
}

func NewRegisterInteractor(
	tenants tenancyport.TenantRepository,
	realms identityport.RealmRepository,
	ids identityport.IdentityRepository,
	creds identityport.CredentialRepository,
	consents identityport.ConsentRepository,
	onetime authport.OneTimeRepository,
	tx txm.TxManager,
	clock clock.Clock,
	audit auditport.Auditor,
) RegisterUsecase {
	return &registerInteractor{
		tenants: tenants, realms: realms, ids: ids, creds: creds,
		consents: consents, onetime: onetime, tx: tx, clock: clock, audit: audit,
	}
}

func (u *registerInteractor) Execute(ctx context.Context, in RegisterInput) (*RegisterOutput, error) {
	if !validate.ValidSlug(in.Tenant) || !validate.ValidCode(in.Realm) || !validate.ValidUsername(in.Username) {
		return nil, apperr.ErrInvalidArgument
	}
	tenant, err := u.tenants.TenantBySlug(ctx, in.Tenant)
	if err != nil || tenant == nil {
		return nil, apperr.ErrNotFound
	}
	realm, err := u.realms.RealmByCode(ctx, tenant.ID, in.Realm)
	if err != nil || realm == nil {
		return nil, apperr.ErrNotFound
	}
	if realm.Kind != "public" || !realm.SelfRegistration {
		return nil, apperr.ErrRegistrationClosed
	}
	if err := realm.PasswordPolicy.Check(in.Password); err != nil {
		return nil, err
	}

	hash, err := kdf.Hash(in.Password)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}

	status := "active"
	if realm.EmailVerification {
		status = "pending"
	}

	out := &RegisterOutput{VerificationRequired: realm.EmailVerification}
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		id, err := u.ids.CreateIdentity(ctx, identitydomain.IdentityCreate{
			TenantID:       tenant.ID,
			RealmID:        realm.ID,
			Username:       in.Username,
			Email:          in.Email,
			AssuranceLevel: 1, // self-asserted, by definition
			Status:         status,
		})
		if err != nil {
			return apperr.ErrConflict.Wrap(err)
		}
		out.IdentityID = id
		if _, err := u.creds.CreateCredential(ctx, credential.CredentialInput{
			IdentityID: id, TenantID: tenant.ID, Kind: "password",
			Secret: hash, Params: []byte("{}"),
		}); err != nil {
			return apperr.ErrInternal.Wrap(err)
		}
		evidence := jsonx.Must(map[string]string{
			"ip": authctx.ClientIP(ctx), "user_agent": authctx.UserAgent(ctx),
		})
		for _, c := range in.Consents {
			if _, err := u.consents.InsertConsent(ctx, tenant.ID, id, c.Purpose, c.PolicyVersion, evidence); err != nil {
				return apperr.ErrInternal.Wrap(err)
			}
		}
		if realm.EmailVerification {
			token, err := secret.New(32)
			if err != nil {
				return apperr.ErrInternal.Wrap(err)
			}
			if _, err := u.onetime.CreateOneTime(ctx, tenant.ID, "email_verify",
				secret.Hash(token), jsonx.Must(map[string]string{"identity_id": id, "tenant_id": tenant.ID}),
				u.clock.Now().Add(emailVerifyTTL)); err != nil {
				return apperr.ErrInternal.Wrap(err)
			}
			out.VerificationToken = token
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: tenant.ID, ActorID: out.IdentityID, ActorKind: "identity",
		Action: "identity.register", Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: jsonx.Must(map[string]string{"realm": in.Realm}),
	})
	return out, nil
}
