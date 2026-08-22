package usecase

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/gsoultan/anubis/internal/crypto/secret"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

const deviceNonceTTL = 60 * time.Second

// deviceInteractor implements DeviceChallengeUsecase and, via Verify(),
// DeviceVerifyUsecase.
type deviceInteractor struct {
	tenants repository.TenantRepository
	realms  repository.RealmRepository
	ids     repository.IdentityRepository
	creds   repository.CredentialRepository
	onetime repository.OneTimeRepository
	est     *sessionEstablisher
	clock   repository.Clock
	audit   repository.Auditor
}

func NewDeviceInteractor(
	tenants repository.TenantRepository,
	realms repository.RealmRepository,
	ids repository.IdentityRepository,
	creds repository.CredentialRepository,
	onetime repository.OneTimeRepository,
	sessions repository.SessionRepository,
	issuer TokenIssuer,
	tx repository.TxManager,
	clock repository.Clock,
	audit repository.Auditor,
) *deviceInteractor {
	return &deviceInteractor{
		tenants: tenants, realms: realms, ids: ids, creds: creds,
		onetime: onetime,
		est:     newSessionEstablisher(sessions, ids, issuer, tx, clock),
		clock:   clock, audit: audit,
	}
}

func (u *deviceInteractor) Execute(ctx context.Context, in DeviceChallengeInput) (*DeviceChallengeOutput, error) {
	tenant, err := u.tenants.TenantBySlug(ctx, in.Tenant)
	if err != nil || tenant == nil {
		return nil, domain.ErrDeviceChallenge
	}
	nonce, err := secret.New(32)
	if err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}
	// The nonce row remembers which credential it was minted for, so a nonce
	// for device A cannot be answered by device B's key.
	if _, err := u.onetime.CreateOneTime(ctx, tenant.ID, "device_challenge",
		secret.Hash(nonce),
		mustJSON(map[string]string{"device_id": in.DeviceID, "realm": in.Realm}),
		u.clock.Now().Add(deviceNonceTTL)); err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}
	return &DeviceChallengeOutput{Nonce: nonce, ExpiresIn: int(deviceNonceTTL / time.Second)}, nil
}

// verifyImpl is the DeviceVerifyUsecase view of the interactor.
type deviceVerifyInteractor struct{ *deviceInteractor }

func (u *deviceInteractor) Verify() DeviceVerifyUsecase {
	return &deviceVerifyInteractor{u}
}

func (u *deviceVerifyInteractor) Execute(ctx context.Context, in DeviceVerifyInput) (*TokenPair, error) {
	tenant, err := u.tenants.TenantBySlug(ctx, in.Tenant)
	if err != nil || tenant == nil {
		return nil, domain.ErrDeviceChallenge
	}
	// Atomic consumption FIRST: a replayed nonce dies here regardless of how
	// valid the signature is.
	_, payload, err := u.onetime.ConsumeOneTime(ctx, "device_challenge", secret.Hash(in.Nonce))
	if err != nil {
		return nil, domain.ErrDeviceChallenge
	}
	var state struct {
		DeviceID string `json:"device_id"`
		Realm    string `json:"realm"`
	}
	if json.Unmarshal(payload, &state) != nil || state.DeviceID != in.KeyID {
		return nil, domain.ErrDeviceChallenge
	}

	c, err := u.credential(ctx, in.KeyID)
	if err != nil {
		return nil, domain.ErrDeviceChallenge
	}
	pub, err := base64.RawURLEncoding.DecodeString(c.Secret)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, domain.ErrDeviceChallenge
	}
	sig, err := base64.RawURLEncoding.DecodeString(in.Signature)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(pub), []byte(in.Nonce), sig) {
		u.audit.Emit(ctx, repository.AuditEvent{
			TenantID: tenant.ID, ActorKind: "identity", Action: "auth.device",
			Result: "deny", Detail: []byte(`{"reason":"bad_signature"}`),
		})
		return nil, domain.ErrDeviceChallenge
	}

	identity, err := u.ids.Identity(ctx, tenant.ID, c.IdentityID)
	if err != nil || identity == nil {
		return nil, domain.ErrDeviceChallenge
	}
	if err := identity.CanAuthenticate(); err != nil {
		return nil, err
	}
	realm, err := u.realms.RealmByCode(ctx, tenant.ID, orDefault(state.Realm, identity.RealmCode))
	if err != nil || realm == nil || !realm.AllowsFactor("device_key") {
		return nil, domain.ErrDeviceChallenge
	}

	u.creds.TouchCredentialUsed(ctx, c.ID, c.SignCounter+1)
	pair, err := u.est.establish(ctx, tenant, realm, identity.ID, in.ClientID, in.DeviceFP,
		[]string{amrDevice})
	if err != nil {
		return nil, err
	}
	u.audit.Emit(ctx, repository.AuditEvent{
		TenantID: tenant.ID, ActorID: identity.ID, ActorKind: "identity",
		Action: "auth.device", Result: "allow", Detail: []byte(`{}`),
	})
	return pair, nil
}

// credential loads the device-key credential by id.
func (u *deviceInteractor) credential(ctx context.Context, id string) (*repository.Credential, error) {
	identityID, tenantID, kind, err := u.creds.CredentialOwner(ctx, id)
	if err != nil || kind != "device_key" {
		return nil, domain.ErrDeviceChallenge
	}
	_ = tenantID
	c, err := u.creds.ActiveCredentialOfKind(ctx, identityID, "device_key")
	if err != nil || c == nil || c.ID != id {
		return nil, domain.ErrDeviceChallenge
	}
	return c, nil
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
