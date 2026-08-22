package device

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authapp "github.com/gsoultan/anubis/internal/auth/app"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/txm"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

const deviceNonceTTL = 60 * time.Second

// deviceInteractor implements DeviceChallengeUsecase and, via Verify(),
// DeviceVerifyUsecase.
type deviceInteractor struct {
	tenants tenancyport.TenantRepository
	realms  identityport.RealmRepository
	ids     identityport.IdentityRepository
	creds   identityport.CredentialRepository
	onetime authport.OneTimeRepository
	est     *authapp.SessionEstablisher
	clock   clock.Clock
	audit   auditport.Auditor
}

func NewDeviceInteractor(
	tenants tenancyport.TenantRepository,
	realms identityport.RealmRepository,
	ids identityport.IdentityRepository,
	creds identityport.CredentialRepository,
	onetime authport.OneTimeRepository,
	sessions authport.SessionRepository,
	issuer authapp.TokenIssuer,
	tx txm.TxManager,
	clock clock.Clock,
	audit auditport.Auditor,
) *deviceInteractor {
	return &deviceInteractor{
		tenants: tenants, realms: realms, ids: ids, creds: creds,
		onetime: onetime,
		est:     authapp.NewSessionEstablisher(sessions, ids, issuer, tx, clock),
		clock:   clock, audit: audit,
	}
}

func (u *deviceInteractor) Execute(ctx context.Context, in DeviceChallengeInput) (*DeviceChallengeOutput, error) {
	tenant, err := u.tenants.TenantBySlug(ctx, in.Tenant)
	if err != nil || tenant == nil {
		return nil, apperr.ErrDeviceChallenge
	}
	nonce, err := secret.New(32)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	// The nonce row remembers which credential it was minted for, so a nonce
	// for device A cannot be answered by device B's key.
	if _, err := u.onetime.CreateOneTime(ctx, tenant.ID, "device_challenge",
		secret.Hash(nonce),
		jsonx.Must(map[string]string{"device_id": in.DeviceID, "realm": in.Realm}),
		u.clock.Now().Add(deviceNonceTTL)); err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	return &DeviceChallengeOutput{Nonce: nonce, ExpiresIn: int(deviceNonceTTL / time.Second)}, nil
}

// verifyImpl is the DeviceVerifyUsecase view of the interactor.
type deviceVerifyInteractor struct{ *deviceInteractor }

func (u *deviceInteractor) Verify() DeviceVerifyUsecase {
	return &deviceVerifyInteractor{u}
}

func (u *deviceVerifyInteractor) Execute(ctx context.Context, in DeviceVerifyInput) (*authapp.TokenPair, error) {
	tenant, err := u.tenants.TenantBySlug(ctx, in.Tenant)
	if err != nil || tenant == nil {
		return nil, apperr.ErrDeviceChallenge
	}
	// Atomic consumption FIRST: a replayed nonce dies here regardless of how
	// valid the signature is.
	_, payload, err := u.onetime.ConsumeOneTime(ctx, "device_challenge", secret.Hash(in.Nonce))
	if err != nil {
		return nil, apperr.ErrDeviceChallenge
	}
	var state struct {
		DeviceID string `json:"device_id"`
		Realm    string `json:"realm"`
	}
	if json.Unmarshal(payload, &state) != nil || state.DeviceID != in.KeyID {
		return nil, apperr.ErrDeviceChallenge
	}

	c, err := u.credential(ctx, in.KeyID)
	if err != nil {
		return nil, apperr.ErrDeviceChallenge
	}
	pub, err := base64.RawURLEncoding.DecodeString(c.Secret)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, apperr.ErrDeviceChallenge
	}
	sig, err := base64.RawURLEncoding.DecodeString(in.Signature)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(pub), []byte(in.Nonce), sig) {
		u.audit.Emit(ctx, auditdomain.AuditEvent{
			TenantID: tenant.ID, ActorKind: "identity", Action: "auth.device",
			Result: "deny", Detail: []byte(`{"reason":"bad_signature"}`),
		})
		return nil, apperr.ErrDeviceChallenge
	}

	identity, err := u.ids.Identity(ctx, tenant.ID, c.IdentityID)
	if err != nil || identity == nil {
		return nil, apperr.ErrDeviceChallenge
	}
	if err := identity.CanAuthenticate(); err != nil {
		return nil, err
	}
	realm, err := u.realms.RealmByCode(ctx, tenant.ID, orDefault(state.Realm, identity.RealmCode))
	if err != nil || realm == nil || !realm.AllowsFactor("device_key") {
		return nil, apperr.ErrDeviceChallenge
	}

	u.creds.TouchCredentialUsed(ctx, c.ID, c.SignCounter+1)
	pair, err := u.est.Establish(ctx, tenant, realm, identity.ID, in.ClientID, in.DeviceFP,
		[]string{authapp.AMRDevice})
	if err != nil {
		return nil, err
	}
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: tenant.ID, ActorID: identity.ID, ActorKind: "identity",
		Action: "auth.device", Result: "allow", Detail: []byte(`{}`),
	})
	return pair, nil
}

// credential loads the device-key credential by id.
func (u *deviceInteractor) credential(ctx context.Context, id string) (*credential.Credential, error) {
	identityID, tenantID, kind, err := u.creds.CredentialOwner(ctx, id)
	if err != nil || kind != "device_key" {
		return nil, apperr.ErrDeviceChallenge
	}
	_ = tenantID
	c, err := u.creds.ActiveCredentialOfKind(ctx, identityID, "device_key")
	if err != nil || c == nil || c.ID != id {
		return nil, apperr.ErrDeviceChallenge
	}
	return c, nil
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
