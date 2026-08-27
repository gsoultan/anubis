package signin

import (
	"context"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authapp "github.com/gsoultan/anubis/internal/auth/app"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/localtoken"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/txm"
	"github.com/gsoultan/anubis/internal/shared/validate"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

const mfaTokenTTL = 60 * time.Second

// enrolGrantTTL is longer than an MFA challenge on purpose: the holder has to
// install an authenticator app, scan a code and type a number, not read six
// digits they already have. Still short enough that a leaked grant is a
// narrow window.
const enrolGrantTTL = 15 * time.Minute

// loginInteractor implements LoginUsecase.
type loginInteractor struct {
	tenants  tenancyport.TenantRepository
	realms   identityport.RealmRepository
	ids      identityport.IdentityRepository
	creds    identityport.CredentialRepository
	sessions authport.SessionRepository
	onetime  authport.OneTimeRepository
	issuer   authapp.TokenIssuer
	ring     *keyring.Manager
	tx       txm.TxManager
	clock    clock.Clock
	audit    auditport.Auditor
}

func NewLoginInteractor(
	tenants tenancyport.TenantRepository,
	realms identityport.RealmRepository,
	ids identityport.IdentityRepository,
	creds identityport.CredentialRepository,
	sessions authport.SessionRepository,
	onetime authport.OneTimeRepository,
	issuer authapp.TokenIssuer,
	ring *keyring.Manager,
	tx txm.TxManager,
	clock clock.Clock,
	audit auditport.Auditor,
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
	if !validate.ValidSlug(in.Tenant) || !validate.ValidUsername(in.Username) ||
		in.Password == "" || len(in.Password) > 512 {
		// Same KDF burn as every other failure: input shape must not be a
		// faster rejection than a wrong password.
		u.burnKDF(in.Password)
		return nil, apperr.ErrInvalidCredentials
	}

	// Resolve tenant, realm, identity, credential — collecting rather than
	// early-returning, so every failure converges on ONE verify call.
	var (
		identity   *identitydomain.Identity
		credential *credential.Credential
		realm      *identitydomain.Realm
		tenant     *tenancydomain.TenantRef
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
		return nil, apperr.ErrInvalidCredentials
	}
	if err := identity.CanAuthenticate(); err != nil {
		u.auditLogin(ctx, tenant, identity, "deny", apperr.AsError(err).Code)
		return nil, err
	}

	if needsRehash {
		if newHash, herr := kdf.Hash(in.Password); herr == nil {
			_ = u.creds.UpdateCredentialSecret(ctx, credential.ID, newHash)
		}
	}

	// Second factor: realm policy demands it and the identity has one
	// enrolled. Enrolment is honoured on its own, before any realm policy —
	// somebody who added an authenticator is asked for it either way.
	enrolled := u.enrolledFactorKinds(ctx, identity.ID)
	methods := allowedOf(realm, enrolled)
	if len(methods) > 0 {
		challenge, err := u.mintMFAChallenge(ctx, tenant, realm, identity, in, methods)
		if err != nil {
			return nil, err
		}
		u.auditLogin(ctx, tenant, identity, "allow", "mfa_challenge")
		return &LoginOutput{MFA: challenge}, nil
	}

	// Required but NOT enrolled. Which of the two answers this gets is the
	// difference between a rollout and a lockout, so it is decided by a date
	// the operator set, not by a flag.
	stance, missing := realm.EnrolmentStanceFor(enrolled, u.clock.Now())
	if stance == identitydomain.EnrolmentOverdue {
		challenge, err := u.mintEnrolmentGrant(ctx, tenant, realm, identity, missing)
		if err != nil {
			return nil, err
		}
		u.auditLogin(ctx, tenant, identity, "deny", "enrolment_required")
		return &LoginOutput{Enrolment: challenge}, nil
	}

	pair, err := u.establishSession(ctx, tenant, realm, identity, in, []string{authapp.AMRPassword})
	if err != nil {
		return nil, err
	}
	u.auditLogin(ctx, tenant, identity, "allow", "password")
	out := &LoginOutput{Tokens: pair}
	if stance == identitydomain.EnrolmentDue {
		// Sign-in worked; this is the warning that it will not next month.
		out.Enrolment = &authapp.EnrolmentChallenge{
			Factors: missing, Deadline: realm.FactorEnrolmentDeadline,
		}
	}
	return out, nil
}

// mintEnrolmentGrant issues the token that makes the refusal actionable.
//
// It is minted only for a member with NONE of the required factors enrolled,
// which is what stops it being a way to replace somebody's authenticator:
// there is nothing to replace. Its holder has already presented the correct
// password — the same bar as an MFA challenge token, and it buys strictly
// less.
func (u *loginInteractor) mintEnrolmentGrant(ctx context.Context, tenant *tenancydomain.TenantRef, realm *identitydomain.Realm, identity *identitydomain.Identity, missing []string) (*authapp.EnrolmentChallenge, error) {
	key, err := u.ring.Ring().ActiveLocal()
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	jti, err := secret.New(16)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	token, err := localtoken.Seal(key.Secret, key.Kid, "enrol_grant", jti, authapp.EnrolmentGrant{
		TenantID:   tenant.ID,
		TenantSlug: tenant.Slug,
		IdentityID: identity.ID,
		RealmID:    realm.ID,
		Factors:    missing,
	}, enrolGrantTTL, u.clock.Now())
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	return &authapp.EnrolmentChallenge{
		Factors:    missing,
		Deadline:   realm.FactorEnrolmentDeadline,
		GrantToken: token,
		ExpiresIn:  int(enrolGrantTTL.Seconds()),
	}, nil
}

// allowedOf keeps only the factors the realm still accepts. A credential row
// can outlive the policy that allowed it.
func allowedOf(realm *identitydomain.Realm, kinds []string) []string {
	var out []string
	for _, k := range kinds {
		if realm.AllowsFactor(k) {
			out = append(out, k)
		}
	}
	return out
}

func (u *loginInteractor) burnKDF(password string) {
	_, _, _ = kdf.Verify(password, kdf.Dummy())
}

// enrolledFactorKinds lists the second factors this identity actually holds.
//
// A factor is demanded when the identity HAS IT ENROLLED — not only when the
// realm requires it. Enrolment is an opt-in to stronger authentication, and
// honouring it only in realms that already mandate MFA would mean a user who
// deliberately added an authenticator still signs in with a password alone.
// That is the entire value of the feature, silently discarded.
//
// The realm's allowed_factors still governs, in allowedOf: a factor the realm
// forbids is not offered even if a credential row survives from before the
// policy changed.
//
// Required-but-unenrolled is handled separately, by the realm's enrolment
// deadline: see Realm.EnrolmentStanceFor and docs/enrolment-rollout.md.
func (u *loginInteractor) enrolledFactorKinds(ctx context.Context, identityID string) []string {
	kinds, err := u.creds.ActiveCredentialKinds(ctx, identityID)
	if err != nil {
		return nil
	}
	var out []string
	for _, k := range kinds {
		switch k {
		case "totp", "device_key":
			out = append(out, k)
		}
	}
	return out
}

func (u *loginInteractor) mintMFAChallenge(ctx context.Context, tenant *tenancydomain.TenantRef, realm *identitydomain.Realm, identity *identitydomain.Identity, in LoginInput, methods []string) (*authapp.MFAChallenge, error) {
	key, err := u.ring.Ring().ActiveLocal()
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	jti, err := secret.New(16)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	now := u.clock.Now()
	token, err := localtoken.Seal(key.Secret, key.Kid, "mfa", jti, authapp.MFAState{
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
		return nil, apperr.ErrInternal.Wrap(err)
	}
	// Single use is enforced server-side: the jti is consumable exactly once.
	if _, err := u.onetime.CreateOneTime(ctx, tenant.ID, "mfa",
		secret.Hash(jti), []byte("{}"), now.Add(mfaTokenTTL)); err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	return &authapp.MFAChallenge{
		MFAToken:  token,
		Methods:   methods,
		ExpiresIn: int(mfaTokenTTL / time.Second),
	}, nil
}

// establishSession creates the session row and mints tokens in one
// transaction — shared by password login, MFA verify and device verify.
func (u *loginInteractor) establishSession(ctx context.Context, tenant *tenancydomain.TenantRef, realm *identitydomain.Realm, identity *identitydomain.Identity, in LoginInput, amr []string) (*authapp.TokenPair, error) {
	var pair *authapp.TokenPair
	err := u.tx.WithinTx(ctx, func(ctx context.Context) error {
		sess, err := u.sessions.CreateSession(ctx, authdomain.SessionInput{
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
			return apperr.ErrInternal.Wrap(err)
		}
		view, err := u.sessions.SessionLive(ctx, sess.ID)
		if err != nil {
			return apperr.ErrInternal.Wrap(err)
		}
		pair, err = u.issuer.Issue(ctx, authapp.IssueInput{
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

func (u *loginInteractor) auditLogin(ctx context.Context, tenant *tenancydomain.TenantRef, identity *identitydomain.Identity, result, detail string) {
	if tenant == nil {
		return // nothing to chain the event to; transport logs carry the rest
	}
	ev := auditdomain.AuditEvent{
		TenantID:  tenant.ID,
		ActorKind: "identity",
		Action:    "auth.login",
		Result:    result,
		IP:        authctx.ClientIP(ctx),
		Detail:    jsonx.Must(map[string]string{"detail": detail}),
	}
	if identity != nil {
		ev.ActorID = identity.ID
	}
	u.audit.Emit(ctx, ev)
}
