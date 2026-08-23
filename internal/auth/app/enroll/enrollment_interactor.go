package enroll

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	credential "github.com/gsoultan/anubis/internal/identity/domain/credential"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/localtoken"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/platform/crypto/totp"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/txm"
)

const (
	enrollTTL         = 10 * time.Minute
	recoveryCodeCount = 10
	maxDeviceKeys     = 10 // bounded: a credential list is attacker-growable
)

// enrollmentInteractor implements EnrollmentUsecase.
type enrollmentInteractor struct {
	issuer  string
	ring    *keyring.Manager
	creds   identityport.CredentialRepository
	ids     identityport.IdentityRepository
	onetime authport.OneTimeRepository
	tx      txm.TxManager
	clock   clock.Clock
	audit   auditport.Auditor
}

func NewEnrollmentInteractor(
	issuer string,
	ring *keyring.Manager,
	creds identityport.CredentialRepository,
	ids identityport.IdentityRepository,
	onetime authport.OneTimeRepository,
	tx txm.TxManager,
	clk clock.Clock,
	audit auditport.Auditor,
) EnrollmentUsecase {
	return &enrollmentInteractor{
		issuer: issuer, ring: ring, creds: creds, ids: ids,
		onetime: onetime, tx: tx, clock: clk, audit: audit,
	}
}

func (u *enrollmentInteractor) BeginTOTP(ctx context.Context) (*TOTPEnrollment, error) {
	p, ok := authctx.From(ctx)
	if !ok || p.Service {
		return nil, apperr.ErrUnauthenticated
	}
	identity, err := u.ids.Identity(ctx, p.TenantID, p.IdentityID)
	if err != nil {
		return nil, apperr.ErrNotFound
	}

	sharedSecret, err := totp.NewSecret()
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	key, err := u.ring.Ring().ActiveLocal()
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	jti, err := secret.New(16)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	now := u.clock.Now()
	token, err := localtoken.Seal(key.Secret, key.Kid, "totp_enroll", jti, enrollmentState{
		IdentityID: p.IdentityID, TenantID: p.TenantID, Secret: sharedSecret,
		Issuer: "Anubis", Account: identity.Username,
	}, enrollTTL, now)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	// Single use, server-side: a replayed enrolment token cannot register a
	// second authenticator behind the user's back.
	if _, err := u.onetime.CreateOneTime(ctx, p.TenantID, "mfa",
		secret.Hash(jti), []byte(`{"purpose":"totp_enroll"}`), now.Add(enrollTTL)); err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	return &TOTPEnrollment{
		ProvisioningURI: totp.ProvisioningURI(sharedSecret, "Anubis", identity.Username,
			totp.DefaultDigits, totp.DefaultStep),
		Secret:          base32Secret(sharedSecret),
		EnrollmentToken: token,
		ExpiresIn:       int(enrollTTL / time.Second),
	}, nil
}

func (u *enrollmentInteractor) ConfirmTOTP(ctx context.Context, enrollmentToken, code string) (*TOTPConfirmation, error) {
	p, ok := authctx.From(ctx)
	if !ok || p.Service {
		return nil, apperr.ErrUnauthenticated
	}
	kid, err := localtoken.Kid(enrollmentToken)
	if err != nil {
		return nil, apperr.ErrMfaInvalid
	}
	key, err := u.ring.Ring().Lookup(kid)
	if err != nil || len(key.Secret) == 0 {
		return nil, apperr.ErrMfaInvalid
	}
	jti, raw, err := localtoken.Open(key.Secret, enrollmentToken, "totp_enroll", u.clock.Now())
	if err != nil {
		return nil, apperr.ErrMfaInvalid
	}
	var state enrollmentState
	if json.Unmarshal(raw, &state) != nil {
		return nil, apperr.ErrMfaInvalid
	}
	// The token belongs to whoever asked for it, not whoever presents it.
	if state.IdentityID != p.IdentityID || state.TenantID != p.TenantID {
		return nil, apperr.ErrMfaInvalid
	}
	if _, _, err := u.onetime.ConsumeOneTime(ctx, "mfa", secret.Hash(jti)); err != nil {
		return nil, apperr.ErrMfaInvalid
	}
	// Proof the authenticator actually holds the secret.
	step, ok := totp.Verify(state.Secret, code, u.clock.Now(), totp.DefaultStep, totp.DefaultDigits, 1)
	if !ok {
		return nil, apperr.ErrMfaInvalid
	}

	master, err := u.ring.Ring().ActiveLocal()
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	out := &TOTPConfirmation{}
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		// Re-enrolment replaces: two active TOTP secrets means one of them is
		// a stale credential nobody can account for.
		if _, err := u.creds.RevokeCredentialsOfKind(ctx, p.IdentityID, "totp"); err != nil {
			return err
		}
		credID, err := u.creds.CreateCredential(ctx, credential.CredentialInput{
			IdentityID: p.IdentityID, TenantID: p.TenantID, Kind: "totp",
			Label:  "authenticator",
			Params: jsonx.Must(map[string]uint64{"last_step": step}),
		})
		if err != nil {
			return err
		}
		// The secret is sealed under the credential's own id, so a row copied
		// to another credential cannot be unsealed.
		sealed, err := keyring.SealSecret(master.Secret, "totp:"+credID, state.Secret)
		if err != nil {
			return err
		}
		if err := u.creds.UpdateCredentialSecret(ctx, credID,
			base64.RawStdEncoding.EncodeToString(sealed)); err != nil {
			return err
		}
		out.CredentialID = credID

		if _, err := u.creds.RevokeCredentialsOfKind(ctx, p.IdentityID, "recovery_code"); err != nil {
			return err
		}
		for i := 0; i < recoveryCodeCount; i++ {
			plain, err := secret.New(10)
			if err != nil {
				return err
			}
			if _, err := u.creds.CreateCredential(ctx, credential.CredentialInput{
				IdentityID: p.IdentityID, TenantID: p.TenantID, Kind: "recovery_code",
				Secret: secret.Hex(secret.Hash(plain)), Label: "recovery",
			}); err != nil {
				return err
			}
			out.RecoveryCodes = append(out.RecoveryCodes, plain)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, Action: "credential.totp_enrolled", Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: []byte(`{}`),
	})
	return out, nil
}

func (u *enrollmentInteractor) EnrollDeviceKey(ctx context.Context, publicKey, label string) (string, error) {
	p, ok := authctx.From(ctx)
	if !ok || p.Service {
		return "", apperr.ErrUnauthenticated
	}
	raw, err := base64.RawURLEncoding.DecodeString(publicKey)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return "", apperr.ErrInvalidArgument.With("public_key", "expected base64url Ed25519 public key")
	}
	// Bounded: without a cap, an attacker with a live session can enrol
	// devices indefinitely and keep access after the password changes.
	existing, err := u.creds.ListCredentials(ctx, p.IdentityID, "device_key")
	if err != nil {
		return "", err
	}
	live := 0
	for _, c := range existing {
		if c.RevokedAt == nil {
			live++
		}
	}
	if live >= maxDeviceKeys {
		return "", apperr.ErrInvalidArgument.With("device_keys", "limit reached; revoke one first")
	}

	credID, err := u.creds.CreateCredential(ctx, credential.CredentialInput{
		IdentityID: p.IdentityID, TenantID: p.TenantID, Kind: "device_key",
		Secret: publicKey, Label: label, Params: []byte(`{}`),
	})
	if err != nil {
		return "", err
	}
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, TargetID: credID,
		Action: "credential.device_key_enrolled", Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: jsonx.Must(map[string]string{"label": label}),
	})
	return credID, nil
}

func base32Secret(b []byte) string {
	const alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	out := make([]byte, 0, (len(b)*8+4)/5)
	var buf, bits uint
	for _, c := range b {
		buf = buf<<8 | uint(c)
		bits += 8
		for bits >= 5 {
			out = append(out, alpha[(buf>>(bits-5))&31])
			bits -= 5
		}
	}
	if bits > 0 {
		out = append(out, alpha[(buf<<(5-bits))&31])
	}
	return string(out)
}
