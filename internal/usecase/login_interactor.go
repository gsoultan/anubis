package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/crypto/kdf"
	"github.com/gsoultan/anubis/internal/crypto/keyring"
	"github.com/gsoultan/anubis/internal/crypto/localtoken"
	"github.com/gsoultan/anubis/internal/crypto/secret"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

const (
	mfaTokenTTL = 60 * time.Second
	// amr values follow RFC 8176.
	amrPassword = "pwd"
	amrOTP      = "otp"
	amrDevice   = "device_key"
)

// loginInteractor implements LoginUsecase.
type loginInteractor struct {
	tenants  repository.TenantRepository
	realms   repository.RealmRepository
	ids      repository.IdentityRepository
	creds    repository.CredentialRepository
	sessions repository.SessionRepository
	onetime  repository.OneTimeRepository
	issuer   TokenIssuer
	ring     *keyring.Manager
	tx       repository.TxManager
	clock    repository.Clock
	audit    repository.Auditor
}

func NewLoginInteractor(
	tenants repository.TenantRepository,
	realms repository.RealmRepository,
	ids repository.IdentityRepository,
	creds repository.CredentialRepository,
	sessions repository.SessionRepository,
	onetime repository.OneTimeRepository,
	issuer TokenIssuer,
	ring *keyring.Manager,
	tx repository.TxManager,
	clock repository.Clock,
	audit repository.Auditor,
) LoginUsecase {
	return &loginInteractor{
		tenants: tenants, realms: realms, ids: ids, creds: creds,
		sessions: sessions, onetime: onetime, issuer: issuer, ring: ring,
		tx: tx, clock: clock, audit: audit,
	}
}

func (u *loginInteractor) Execute(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	if in.Realm == "" {
		in.Realm = "internal"
	}
	if !domain.ValidSlug(in.Tenant) || !domain.ValidUsername(in.Username) ||
		in.Password == "" || len(in.Password) > 512 {
		// Same KDF burn as every other failure: input shape must not be a
		// faster rejection than a wrong password.
		u.burnKDF(in.Password)
		return nil, domain.ErrInvalidCredentials
	}

	// Resolve tenant, realm, identity, credential — collecting rather than
	// early-returning, so every failure converges on ONE verify call.
	var (
		identity   *domain.Identity
		credential *repository.Credential
		realm      *domain.Realm
		tenant     *repository.TenantRef
	)
	tenant, err := u.tenants.TenantBySlug(ctx, in.Tenant)
	if err == nil && tenant != nil {
		realm, err = u.realms.RealmByCode(ctx, tenant.ID, in.Realm)
		if err == nil && realm != nil && realm.AllowsFactor("password") {
			identity, _ = u.ids.IdentityForLogin(ctx, tenant.ID, realm.ID, in.Username)
			if identity != nil {
				credential, _ = u.creds.PasswordCredential(ctx, identity.ID)
			}
		}
	}

	hash := kdf.Dummy()
	if credential != nil && credential.Secret != "" {
		hash = credential.Secret
	}
	ok, needsRehash, kerr := kdf.Verify(in.Password, hash)
	if kerr != nil || credential == nil || identity == nil || !ok {
		u.auditLogin(ctx, tenant, identity, "deny", "invalid_credentials")
		return nil, domain.ErrInvalidCredentials
	}
	if err := identity.CanAuthenticate(); err != nil {
		u.auditLogin(ctx, tenant, identity, "deny", domain.AsError(err).Code)
		return nil, err
	}

	if needsRehash {
		if newHash, herr := kdf.Hash(in.Password); herr == nil {
			_ = u.creds.UpdateCredentialSecret(ctx, credential.ID, newHash)
		}
	}

	// Second factor: realm policy demands it and the identity has one
	// enrolled. Required-but-unenrolled lets the login through (the
	// enrollment gap) — visible in amr, closed by admin policy.
	methods := u.availableSecondFactors(ctx, realm, identity.ID)
	if len(methods) > 0 {
		challenge, err := u.mintMFAChallenge(ctx, tenant, realm, identity, in, methods)
		if err != nil {
			return nil, err
		}
		u.auditLogin(ctx, tenant, identity, "allow", "mfa_challenge")
		return &LoginOutput{MFA: challenge}, nil
	}

	pair, err := u.establishSession(ctx, tenant, realm, identity, in, []string{amrPassword})
	if err != nil {
		return nil, err
	}
	u.auditLogin(ctx, tenant, identity, "allow", "password")
	return &LoginOutput{Tokens: pair}, nil
}

func (u *loginInteractor) burnKDF(password string) {
	_, _, _ = kdf.Verify(password, kdf.Dummy())
}

func (u *loginInteractor) availableSecondFactors(ctx context.Context, realm *domain.Realm, identityID string) []string {
	wantTOTP := realm.RequiresFactor("totp")
	wantDevice := realm.RequiresFactor("device_key")
	if !wantTOTP && !wantDevice {
		return nil
	}
	kinds, err := u.creds.ActiveCredentialKinds(ctx, identityID)
	if err != nil {
		return nil
	}
	var out []string
	for _, k := range kinds {
		if (k == "totp" && wantTOTP) || (k == "device_key" && wantDevice) {
			out = append(out, k)
		}
	}
	return out
}

func (u *loginInteractor) mintMFAChallenge(ctx context.Context, tenant *repository.TenantRef, realm *domain.Realm, identity *domain.Identity, in LoginInput, methods []string) (*MFAChallenge, error) {
	key, err := u.ring.Ring().ActiveLocal()
	if err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}
	jti, err := secret.New(16)
	if err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}
	now := u.clock.Now()
	token, err := localtoken.Seal(key.Secret, key.Kid, "mfa", jti, mfaState{
		TenantID:   tenant.ID,
		TenantSlug: tenant.Slug,
		IdentityID: identity.ID,
		RealmID:    realm.ID,
		ClientID:   in.ClientID,
		DeviceFP:   in.DeviceFP,
		Methods:    methods,
		IP:         authctx.ClientIP(ctx),
		UserAgent:  authctx.UserAgent(ctx),
	}, mfaTokenTTL, now)
	if err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}
	// Single use is enforced server-side: the jti is consumable exactly once.
	if _, err := u.onetime.CreateOneTime(ctx, tenant.ID, "mfa",
		secret.Hash(jti), []byte("{}"), now.Add(mfaTokenTTL)); err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}
	return &MFAChallenge{
		MFAToken:  token,
		Methods:   methods,
		ExpiresIn: int(mfaTokenTTL / time.Second),
	}, nil
}

// establishSession creates the session row and mints tokens in one
// transaction — shared by password login, MFA verify and device verify.
func (u *loginInteractor) establishSession(ctx context.Context, tenant *repository.TenantRef, realm *domain.Realm, identity *domain.Identity, in LoginInput, amr []string) (*TokenPair, error) {
	var pair *TokenPair
	err := u.tx.WithinTx(ctx, func(ctx context.Context) error {
		sess, err := u.sessions.CreateSession(ctx, repository.SessionInput{
			IdentityID:   identity.ID,
			TenantID:     tenant.ID,
			AMR:          amr,
			DeviceFP:     in.DeviceFP,
			IP:           authctx.ClientIP(ctx),
			UserAgent:    authctx.UserAgent(ctx),
			ActiveScopes: []byte("{}"),
			ExpiresAt:    u.clock.Now().Add(realm.SessionTTL),
		})
		if err != nil {
			return domain.ErrInternal.Wrap(err)
		}
		view, err := u.sessions.SessionLive(ctx, sess.ID)
		if err != nil {
			return domain.ErrInternal.Wrap(err)
		}
		pair, err = u.issuer.Issue(ctx, IssueInput{
			Session:    view,
			TenantSlug: tenant.Slug,
			ClientID:   in.ClientID,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	u.ids.TouchLastLogin(ctx, identity.ID)
	return pair, nil
}

func (u *loginInteractor) auditLogin(ctx context.Context, tenant *repository.TenantRef, identity *domain.Identity, result, detail string) {
	if tenant == nil {
		return // nothing to chain the event to; transport logs carry the rest
	}
	ev := repository.AuditEvent{
		TenantID:  tenant.ID,
		ActorKind: "identity",
		Action:    "auth.login",
		Result:    result,
		IP:        authctx.ClientIP(ctx),
		Detail:    mustJSON(map[string]string{"detail": detail}),
	}
	if identity != nil {
		ev.ActorID = identity.ID
	}
	u.audit.Emit(ctx, ev)
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}
