package usecase

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/crypto/kdf"
	"github.com/gsoultan/anubis/internal/crypto/secret"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

const emailVerifyTTL = 24 * time.Hour

// registerInteractor implements RegisterUsecase.
type registerInteractor struct {
	tenants  repository.TenantRepository
	realms   repository.RealmRepository
	ids      repository.IdentityRepository
	creds    repository.CredentialRepository
	consents repository.ConsentRepository
	onetime  repository.OneTimeRepository
	tx       repository.TxManager
	clock    repository.Clock
	audit    repository.Auditor
}

func NewRegisterInteractor(
	tenants repository.TenantRepository,
	realms repository.RealmRepository,
	ids repository.IdentityRepository,
	creds repository.CredentialRepository,
	consents repository.ConsentRepository,
	onetime repository.OneTimeRepository,
	tx repository.TxManager,
	clock repository.Clock,
	audit repository.Auditor,
) RegisterUsecase {
	return &registerInteractor{
		tenants: tenants, realms: realms, ids: ids, creds: creds,
		consents: consents, onetime: onetime, tx: tx, clock: clock, audit: audit,
	}
}

func (u *registerInteractor) Execute(ctx context.Context, in RegisterInput) (*RegisterOutput, error) {
	if !domain.ValidSlug(in.Tenant) || !domain.ValidCode(in.Realm) || !domain.ValidUsername(in.Username) {
		return nil, domain.ErrInvalidArgument
	}
	tenant, err := u.tenants.TenantBySlug(ctx, in.Tenant)
	if err != nil || tenant == nil {
		return nil, domain.ErrNotFound
	}
	realm, err := u.realms.RealmByCode(ctx, tenant.ID, in.Realm)
	if err != nil || realm == nil {
		return nil, domain.ErrNotFound
	}
	if realm.Kind != "public" || !realm.SelfRegistration {
		return nil, domain.ErrRegistrationClosed
	}
	if err := realm.PasswordPolicy.Check(in.Password); err != nil {
		return nil, err
	}

	hash, err := kdf.Hash(in.Password)
	if err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}

	status := "active"
	if realm.EmailVerification {
		status = "pending"
	}

	out := &RegisterOutput{VerificationRequired: realm.EmailVerification}
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		id, err := u.ids.CreateIdentity(ctx, repository.IdentityCreate{
			TenantID:       tenant.ID,
			RealmID:        realm.ID,
			Username:       in.Username,
			Email:          in.Email,
			AssuranceLevel: 1, // self-asserted, by definition
			Status:         status,
		})
		if err != nil {
			return domain.ErrConflict.Wrap(err)
		}
		out.IdentityID = id
		if _, err := u.creds.CreateCredential(ctx, repository.CredentialInput{
			IdentityID: id, TenantID: tenant.ID, Kind: "password",
			Secret: hash, Params: []byte("{}"),
		}); err != nil {
			return domain.ErrInternal.Wrap(err)
		}
		evidence := mustJSON(map[string]string{
			"ip": authctx.ClientIP(ctx), "user_agent": authctx.UserAgent(ctx),
		})
		for _, c := range in.Consents {
			if _, err := u.consents.InsertConsent(ctx, tenant.ID, id, c.Purpose, c.PolicyVersion, evidence); err != nil {
				return domain.ErrInternal.Wrap(err)
			}
		}
		if realm.EmailVerification {
			token, err := secret.New(32)
			if err != nil {
				return domain.ErrInternal.Wrap(err)
			}
			if _, err := u.onetime.CreateOneTime(ctx, tenant.ID, "email_verify",
				secret.Hash(token), mustJSON(map[string]string{"identity_id": id, "tenant_id": tenant.ID}),
				u.clock.Now().Add(emailVerifyTTL)); err != nil {
				return domain.ErrInternal.Wrap(err)
			}
			out.VerificationToken = token
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	u.audit.Emit(ctx, repository.AuditEvent{
		TenantID: tenant.ID, ActorID: out.IdentityID, ActorKind: "identity",
		Action: "identity.register", Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: mustJSON(map[string]string{"realm": in.Realm}),
	})
	return out, nil
}
