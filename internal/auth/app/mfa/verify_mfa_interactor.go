package mfa

import (
	"context"
	"encoding/base64"
	"encoding/json"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authapp "github.com/gsoultan/anubis/internal/auth/app"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/localtoken"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/platform/crypto/totp"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/txm"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// verifyMfaInteractor implements VerifyMfaUsecase for TOTP.
type verifyMfaInteractor struct {
	ring    *keyring.Manager
	onetime authport.OneTimeRepository
	creds   identityport.CredentialRepository
	ids     identityport.IdentityRepository
	realms  identityport.RealmRepository
	tenants tenancyport.TenantRepository
	est     *authapp.SessionEstablisher
	clock   clock.Clock
	audit   auditport.Auditor
}

func NewVerifyMfaInteractor(
	ring *keyring.Manager,
	onetime authport.OneTimeRepository,
	creds identityport.CredentialRepository,
	ids identityport.IdentityRepository,
	realms identityport.RealmRepository,
	tenants tenancyport.TenantRepository,
	sessions authport.SessionRepository,
	issuer authapp.TokenIssuer,
	tx txm.TxManager,
	clock clock.Clock,
	audit auditport.Auditor,
) VerifyMfaUsecase {
	return &verifyMfaInteractor{
		ring: ring, onetime: onetime, creds: creds, ids: ids,
		realms: realms, tenants: tenants,
		est:   authapp.NewSessionEstablisher(sessions, ids, issuer, tx, clock),
		clock: clock, audit: audit,
	}
}

func (u *verifyMfaInteractor) Execute(ctx context.Context, in VerifyMfaInput) (*authapp.TokenPair, error) {
	state, err := u.openState(ctx, in.MFAToken)
	if err != nil {
		return nil, err
	}
	if in.Method != "totp" {
		return nil, apperr.ErrMfaInvalid
	}

	cred, err := u.creds.ActiveCredentialOfKind(ctx, state.IdentityID, "totp")
	if err != nil || cred == nil {
		return nil, apperr.ErrMfaInvalid
	}
	sealed, err := base64.RawStdEncoding.DecodeString(cred.Secret)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	// Open under the key this secret was SEALED with, not whichever is active
	// now. Using the active one meant `keys promote local` locked out every
	// enrolled identity: the unseal fails, so every second factor in the
	// installation stops working at once, and the wrapped error blamed the
	// MASTER key ("wrong master key?") when the master was never involved.
	sharedSecret, usedKid, err := keyring.OpenNamedSecret(
		u.ring.Ring(), cred.SecretKid, "totp:"+cred.ID, sealed)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	// An enrolment from before the kid was recorded: write down the answer so
	// the next rotation does not have to guess it.
	if cred.SecretKid == "" {
		if err := u.creds.UpdateCredentialSecret(ctx, cred.ID, cred.Secret, usedKid); err != nil {
			return nil, apperr.ErrInternal.Wrap(err)
		}
	}

	step, ok := totp.Verify(sharedSecret, in.Code, u.clock.Now(), totp.DefaultStep, totp.DefaultDigits, 1)
	// Single use, decided by the DATABASE. This was a Go-side comparison
	// against the last accepted step followed by an unconditional write, and
	// every concurrent presentation of one code passed the comparison —
	// measured at eight of eight. The guard is now in the WHERE clause, so
	// exactly one caller can win, and losing is a replay.
	fresh := false
	if ok {
		var aerr error
		fresh, aerr = u.creds.AdvanceCredentialStep(ctx, cred.ID, step)
		if aerr != nil {
			// A guard that cannot be recorded has not been applied. Refusing
			// is the only safe answer: the alternative is accepting a code
			// that nothing prevents being replayed.
			return nil, apperr.ErrInternal.Wrap(aerr)
		}
	}
	if !ok || !fresh {
		u.audit.Emit(ctx, auditdomain.AuditEvent{
			TenantID: state.TenantID, ActorID: state.IdentityID,
			ActorKind: "identity", Action: "auth.mfa", Result: "deny",
			IP: state.IP, Detail: jsonx.Must(map[string]string{"method": "totp"}),
		})
		return nil, apperr.ErrMfaInvalid
	}
	tenant, err := u.tenants.TenantByID(ctx, state.TenantID)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	identity, err := u.ids.Identity(ctx, state.TenantID, state.IdentityID)
	if err != nil || identity == nil {
		return nil, apperr.ErrMfaInvalid
	}
	if err := identity.CanAuthenticate(); err != nil {
		return nil, err
	}
	realm, err := u.realms.RealmByID(ctx, state.RealmID)
	if err != nil || realm == nil {
		return nil, apperr.ErrMfaInvalid
	}

	pair, err := u.est.Establish(ctx, tenant, realm, identity.ID, state.ClientID, state.DeviceFP,
		[]string{authapp.AMRPassword, authapp.AMROTP})
	if err != nil {
		return nil, err
	}
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: state.TenantID, ActorID: state.IdentityID,
		ActorKind: "identity", Action: "auth.mfa", Result: "allow",
		IP: state.IP, Detail: jsonx.Must(map[string]string{"method": "totp"}),
	})
	return pair, nil
}

// openState authenticates the MFA token and consumes its single use.
func (u *verifyMfaInteractor) openState(ctx context.Context, token string) (*authapp.MFAState, error) {
	kid, err := localtoken.Kid(token)
	if err != nil {
		return nil, apperr.ErrMfaInvalid
	}
	key, err := u.ring.Ring().Lookup(kid)
	if err != nil || len(key.Secret) == 0 {
		return nil, apperr.ErrMfaInvalid
	}
	jti, raw, err := localtoken.Open(key.Secret, token, "mfa", u.clock.Now())
	if err != nil {
		return nil, apperr.ErrMfaInvalid
	}
	if _, _, err := u.onetime.ConsumeOneTime(ctx, "mfa", secret.Hash(jti)); err != nil {
		// Second presentation: single use already spent.
		return nil, apperr.ErrMfaInvalid
	}
	var state authapp.MFAState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, apperr.ErrMfaInvalid
	}
	return &state, nil
}

// stepIsFresh enforces monotonic TOTP acceptance.
