package mfa

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"

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
	key, err := u.ring.Ring().ActiveLocal()
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	sealed, err := base64.RawStdEncoding.DecodeString(cred.Secret)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	sharedSecret, err := keyring.OpenSecret(key.Secret, "totp:"+cred.ID, sealed)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}

	step, ok := totp.Verify(sharedSecret, in.Code, u.clock.Now(), totp.DefaultStep, totp.DefaultDigits, 1)
	if !ok || !u.stepIsFresh(cred.Params, step) {
		u.audit.Emit(ctx, auditdomain.AuditEvent{
			TenantID: state.TenantID, ActorID: state.IdentityID,
			ActorKind: "identity", Action: "auth.mfa", Result: "deny",
			IP: state.IP, Detail: jsonx.Must(map[string]string{"method": "totp"}),
		})
		return nil, apperr.ErrMfaInvalid
	}
	// Replay guard: persist the accepted step; a code may be accepted once.
	_ = u.creds.UpdateCredentialParams(ctx, cred.ID,
		jsonx.Must(map[string]uint64{"last_step": step}))

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
func (u *verifyMfaInteractor) stepIsFresh(params []byte, step uint64) bool {
	var p map[string]json.RawMessage
	if err := json.Unmarshal(params, &p); err != nil {
		return true
	}
	raw, ok := p["last_step"]
	if !ok {
		return true
	}
	last, err := strconv.ParseUint(string(raw), 10, 64)
	if err != nil {
		return true
	}
	return step > last
}
