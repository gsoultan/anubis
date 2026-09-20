package signin

import (
	"context"
	"time"

	authapp "github.com/gsoultan/anubis/internal/auth/app"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/localtoken"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/txm"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

const mfaTokenTTL = 60 * time.Second

// enrolGrantTTL is longer than an MFA challenge on purpose: the holder has to
// install an authenticator app, scan a code and type a number, not read six
// digits they already have. Still short enough that a leaked grant is a
// narrow window.
const enrolGrantTTL = 15 * time.Minute

// loginInteractor implements LoginUsecase.
//
// It owns what the API door does with an authenticated password — mint a
// token pair, an MFA challenge or an enrolment grant. Deciding WHETHER to
// is not its job: that is PasswordAuthenticator's, shared with the hosted
// sign-in page so the two doors cannot answer differently.
type loginInteractor struct {
	auth     *PasswordAuthenticator
	ids      identityport.IdentityRepository
	sessions authport.SessionRepository
	onetime  authport.OneTimeRepository
	issuer   authapp.TokenIssuer
	ring     *keyring.Manager
	tx       txm.TxManager
	clock    clock.Clock
}

func NewLoginInteractor(
	auth *PasswordAuthenticator,
	ids identityport.IdentityRepository,
	sessions authport.SessionRepository,
	onetime authport.OneTimeRepository,
	issuer authapp.TokenIssuer,
	ring *keyring.Manager,
	tx txm.TxManager,
	clock clock.Clock,
) LoginUsecase {
	return &loginInteractor{
		auth: auth, ids: ids, sessions: sessions, onetime: onetime,
		issuer: issuer, ring: ring, tx: tx, clock: clock,
	}
}

func (u *loginInteractor) Execute(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	d := u.auth.Authenticate(ctx, in, SurfaceAPI)
	switch d.Step {
	case StepDeny:
		return nil, d.Err

	case StepFactor:
		challenge, err := u.mintMFAChallenge(ctx, d.Tenant, d.Realm, d.Identity, in, d.Methods)
		if err != nil {
			return nil, err
		}
		return &LoginOutput{MFA: challenge}, nil

	case StepEnrol:
		challenge, err := u.mintEnrolmentGrant(ctx, d.Tenant, d.Realm, d.Identity, d.Missing)
		if err != nil {
			return nil, err
		}
		return &LoginOutput{Enrolment: challenge}, nil
	}

	pair, err := u.establishSession(ctx, d.Tenant, d.Realm, d.Identity, in, []string{authapp.AMRPassword})
	if err != nil {
		return nil, err
	}
	u.auth.AuditLogin(ctx, d.Tenant, d.Identity, SurfaceAPI, "allow", "password", "")
	out := &LoginOutput{Tokens: pair}
	if d.Due {
		// Sign-in worked; this is the warning that it will not next month.
		out.Enrolment = &authapp.EnrolmentChallenge{
			Factors: d.Missing, Deadline: d.Deadline,
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
