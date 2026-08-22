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

// identityAdminInteractor implements IdentityAdminUsecase.
type identityAdminInteractor struct {
	guard    *adminGuard
	dir      repository.IdentityDirectoryRepository
	ids      repository.IdentityRepository
	creds    repository.CredentialRepository
	realms   repository.RealmRepository
	catalog  repository.RealmAdminRepository
	consents repository.ConsentRepository
	sessions repository.SessionRepository
	refresh  repository.RefreshRepository
	tenants  repository.TenantRepository
	tx       repository.TxManager
	clock    repository.Clock
	audit    repository.Auditor
}

func NewIdentityAdminInteractor(
	authz repository.AuthzRepository,
	dir repository.IdentityDirectoryRepository,
	ids repository.IdentityRepository,
	creds repository.CredentialRepository,
	realms repository.RealmRepository,
	catalog repository.RealmAdminRepository,
	consents repository.ConsentRepository,
	sessions repository.SessionRepository,
	refresh repository.RefreshRepository,
	tenants repository.TenantRepository,
	tx repository.TxManager,
	clock repository.Clock,
	audit repository.Auditor,
) IdentityAdminUsecase {
	return &identityAdminInteractor{
		guard: newAdminGuard(authz), dir: dir, ids: ids, creds: creds,
		realms: realms, catalog: catalog, consents: consents,
		sessions: sessions, refresh: refresh, tenants: tenants,
		tx: tx, clock: clock, audit: audit,
	}
}

func (u *identityAdminInteractor) emit(ctx context.Context, p *authctx.Principal, action, targetID string, detail map[string]string) {
	u.audit.Emit(ctx, repository.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, TargetID: targetID, Action: action,
		Result: "allow", IP: authctx.ClientIP(ctx), Detail: mustJSON(detail),
	})
}

func (u *identityAdminInteractor) ListIdentities(ctx context.Context, f repository.IdentityFilter) ([]repository.IdentityRecord, string, error) {
	p, err := u.guard.require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, "", err
	}
	// The wire filter names the realm by CODE; the query wants its id.
	if f.RealmID != "" && len(f.RealmID) != 36 {
		realm, rerr := u.realms.RealmByCode(ctx, p.TenantID, f.RealmID)
		if rerr != nil {
			return nil, "", domain.ErrNotFound.With("realm", f.RealmID)
		}
		f.RealmID = realm.ID
	}
	list, err := u.dir.ListIdentities(ctx, p.TenantID, f)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if f.Limit > 0 && len(list) == f.Limit {
		next = list[len(list)-1].ID
	}
	return list, next, nil
}

func (u *identityAdminInteractor) GetIdentity(ctx context.Context, id string) (*repository.IdentityRecord, []repository.CredentialInfo, error) {
	p, err := u.guard.require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, nil, err
	}
	rec, err := u.dir.IdentityRecordByID(ctx, p.TenantID, id)
	if err != nil {
		return nil, nil, err
	}
	creds, err := u.creds.ListCredentials(ctx, id, "")
	if err != nil {
		return nil, nil, err
	}
	return rec, creds, nil
}

func (u *identityAdminInteractor) CreateIdentity(ctx context.Context, in AdminCreateIdentity) (*repository.IdentityRecord, error) {
	p, err := u.guard.require(ctx, "anubis:identity:write")
	if err != nil {
		return nil, err
	}
	if !domain.ValidUsername(in.Username) || !domain.ValidCode(in.Realm) {
		return nil, domain.ErrInvalidArgument
	}
	realm, err := u.realms.RealmByCode(ctx, p.TenantID, in.Realm)
	if err != nil {
		return nil, domain.ErrNotFound.With("realm", in.Realm)
	}
	assurance := in.AssuranceLevel
	if assurance == 0 {
		assurance = realm.MinAssurance
	}
	categoryID := ""
	if in.Category != "" {
		cat, cerr := u.catalog.RealmCategoryByCode(ctx, realm.ID, in.Category)
		if cerr != nil {
			return nil, domain.ErrInvalidArgument.With("category", in.Category)
		}
		categoryID = cat.ID
	}
	var id string
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		var cerr error
		id, cerr = u.ids.CreateIdentity(ctx, repository.IdentityCreate{
			TenantID: p.TenantID, RealmID: realm.ID, Username: in.Username,
			Email: in.Email, ExternalRef: in.ExternalRef,
			AssuranceLevel: assurance, CategoryID: categoryID, Status: "active",
		})
		if cerr != nil {
			return cerr
		}
		if in.Password != "" {
			if perr := realm.PasswordPolicy.Check(in.Password); perr != nil {
				return perr
			}
			hash, herr := kdf.Hash(in.Password)
			if herr != nil {
				return domain.ErrInternal.Wrap(herr)
			}
			_, cerr = u.creds.CreateCredential(ctx, repository.CredentialInput{
				IdentityID: id, TenantID: p.TenantID, Kind: "password", Secret: hash,
			})
		}
		return cerr
	})
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "identity.create", id, map[string]string{"realm": in.Realm, "username": in.Username})
	return u.dir.IdentityRecordByID(ctx, p.TenantID, id)
}

// DisableIdentity is immediate, complete deprovisioning: authorize() gates on
// identity state, so no grant needs touching — and every live session dies.
func (u *identityAdminInteractor) DisableIdentity(ctx context.Context, id, reason string) error {
	p, err := u.guard.require(ctx, "anubis:identity:write")
	if err != nil {
		return err
	}
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := u.ids.DisableIdentity(ctx, p.TenantID, id); err != nil {
			return err
		}
		revoked, err := u.sessions.RevokeAllSessions(ctx, p.TenantID, id, "identity_disabled")
		if err != nil {
			return err
		}
		sids := make([]string, len(revoked))
		for i, s := range revoked {
			sids[i] = s.ID
		}
		if len(sids) > 0 {
			if _, err := u.refresh.RevokeRefreshBySessions(ctx, sids); err != nil {
				return err
			}
		}
		_, err = u.ids.BumpTokenEpoch(ctx, p.TenantID, id)
		return err
	})
	if err != nil {
		return err
	}
	u.emit(ctx, p, "identity.disable", id, map[string]string{"reason": reason})
	return nil
}

func (u *identityAdminInteractor) EnableIdentity(ctx context.Context, id string) error {
	p, err := u.guard.require(ctx, "anubis:identity:write")
	if err != nil {
		return err
	}
	if err := u.ids.EnableIdentity(ctx, p.TenantID, id); err != nil {
		return err
	}
	u.emit(ctx, p, "identity.enable", id, nil)
	return nil
}

func (u *identityAdminInteractor) BumpTokenEpoch(ctx context.Context, id string) (int, error) {
	p, err := u.guard.require(ctx, "anubis:identity:write")
	if err != nil {
		return 0, err
	}
	epoch, err := u.ids.BumpTokenEpoch(ctx, p.TenantID, id)
	if err != nil {
		return 0, err
	}
	u.emit(ctx, p, "identity.epoch_bump", id, nil)
	return epoch, nil
}

func (u *identityAdminInteractor) SetPassword(ctx context.Context, id, password string) error {
	p, err := u.guard.require(ctx, "anubis:credential:write")
	if err != nil {
		return err
	}
	identity, err := u.ids.Identity(ctx, p.TenantID, id)
	if err != nil {
		return domain.ErrNotFound
	}
	realm, err := u.realms.RealmByID(ctx, identity.RealmID)
	if err != nil {
		return domain.ErrInternal.Wrap(err)
	}
	if err := realm.PasswordPolicy.Check(password); err != nil {
		return err
	}
	hash, err := kdf.Hash(password)
	if err != nil {
		return domain.ErrInternal.Wrap(err)
	}
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := u.creds.RevokeCredentialsOfKind(ctx, id, "password"); err != nil {
			return err
		}
		_, err := u.creds.CreateCredential(ctx, repository.CredentialInput{
			IdentityID: id, TenantID: p.TenantID, Kind: "password", Secret: hash,
		})
		return err
	})
	if err != nil {
		return err
	}
	u.emit(ctx, p, "credential.password_set", id, nil)
	return nil
}

func (u *identityAdminInteractor) LinkIdentities(ctx context.Context, primaryID, secondaryID, method, evidenceJSON string) error {
	p, err := u.guard.require(ctx, "anubis:identity:write")
	if err != nil {
		return err
	}
	if err := u.ids.LinkIdentities(ctx, p.TenantID, primaryID, secondaryID,
		p.IdentityID, method, jsonOrEmpty(evidenceJSON)); err != nil {
		return err
	}
	u.emit(ctx, p, "identity.link", primaryID, map[string]string{"secondary": secondaryID, "method": method})
	return nil
}

func (u *identityAdminInteractor) RequestErasure(ctx context.Context, id string) error {
	p, err := u.guard.require(ctx, "anubis:identity:write")
	if err != nil {
		return err
	}
	if err := u.ids.RequestErasure(ctx, p.TenantID, id); err != nil {
		return err
	}
	u.emit(ctx, p, "identity.erasure_requested", id, nil)
	return nil
}

func (u *identityAdminInteractor) ListCredentials(ctx context.Context, identityID string) ([]repository.CredentialInfo, error) {
	if _, err := u.guard.require(ctx, "anubis:identity:read"); err != nil {
		return nil, err
	}
	return u.creds.ListCredentials(ctx, identityID, "")
}

func (u *identityAdminInteractor) RevokeCredential(ctx context.Context, credentialID string) error {
	p, err := u.guard.require(ctx, "anubis:credential:write")
	if err != nil {
		return err
	}
	if err := u.creds.RevokeCredential(ctx, p.TenantID, credentialID); err != nil {
		return err
	}
	u.emit(ctx, p, "credential.revoke", credentialID, nil)
	return nil
}

func (u *identityAdminInteractor) CreateAPIKey(ctx context.Context, identityID, label string, expiresAt int64) (string, string, string, error) {
	p, err := u.guard.require(ctx, "anubis:credential:write")
	if err != nil {
		return "", "", "", err
	}
	full, lookup, hash, err := secret.NewAPIKey()
	if err != nil {
		return "", "", "", domain.ErrInternal.Wrap(err)
	}
	var exp *time.Time
	if expiresAt > 0 {
		t := time.Unix(expiresAt, 0)
		exp = &t
	}
	credID, err := u.creds.CreateCredential(ctx, repository.CredentialInput{
		IdentityID: identityID, TenantID: p.TenantID, Kind: "api_key",
		Secret: hexEncode(hash), LookupKey: lookup, Label: label, ExpiresAt: exp,
	})
	if err != nil {
		return "", "", "", err
	}
	u.emit(ctx, p, "credential.api_key_create", identityID, map[string]string{"prefix": lookup})
	return full, lookup, credID, nil
}

func (u *identityAdminInteractor) ListAPIKeys(ctx context.Context, identityID string) ([]repository.CredentialInfo, error) {
	if _, err := u.guard.require(ctx, "anubis:identity:read"); err != nil {
		return nil, err
	}
	return u.creds.ListCredentials(ctx, identityID, "api_key")
}

func (u *identityAdminInteractor) ListConsents(ctx context.Context, identityID string) ([]repository.ConsentRecord, error) {
	p, err := u.guard.require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	return u.consents.ListConsents(ctx, p.TenantID, identityID)
}

func (u *identityAdminInteractor) RecordConsent(ctx context.Context, identityID, purpose, policyVersion, evidenceJSON string) (*repository.ConsentRecord, error) {
	p, err := u.guard.require(ctx, "anubis:consent:write")
	if err != nil {
		return nil, err
	}
	rec, err := u.consents.InsertConsent(ctx, p.TenantID, identityID, purpose,
		policyVersion, jsonOrEmpty(evidenceJSON))
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "consent.record", identityID, map[string]string{"purpose": purpose})
	return rec, nil
}

func (u *identityAdminInteractor) WithdrawConsent(ctx context.Context, consentID string) error {
	p, err := u.guard.require(ctx, "anubis:consent:write")
	if err != nil {
		return err
	}
	if err := u.consents.WithdrawConsent(ctx, p.TenantID, consentID); err != nil {
		return err
	}
	u.emit(ctx, p, "consent.withdraw", consentID, nil)
	return nil
}

func jsonOrEmpty(s string) []byte {
	if s == "" {
		return []byte("{}")
	}
	return []byte(s)
}

func hexEncode(b []byte) string {
	const hexdig = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2] = hexdig[c>>4]
		out[i*2+1] = hexdig[c&0xf]
	}
	return string(out)
}
