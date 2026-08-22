package usecase

import (
	"context"
	"encoding/json"

	"github.com/gsoultan/anubis/internal/crypto/keyring"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
	"github.com/gsoultan/anubis/pkg/anubis"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

// introspectInteractor implements IntrospectUsecase.
type introspectInteractor struct {
	issuer   string
	ring     *keyring.Manager
	sessions repository.SessionRepository
	tenants  repository.TenantRepository
	clock    repository.Clock
}

func NewIntrospectInteractor(
	issuer string,
	ring *keyring.Manager,
	sessions repository.SessionRepository,
	tenants repository.TenantRepository,
	clock repository.Clock,
) IntrospectUsecase {
	return &introspectInteractor{issuer: issuer, ring: ring, sessions: sessions, tenants: tenants, clock: clock}
}

var inactive = &IntrospectResult{Active: false}

func (u *introspectInteractor) Execute(ctx context.Context, token string) (*IntrospectResult, error) {
	claims, err := u.verify(token)
	if err != nil {
		return inactive, nil // an invalid token is not an error, it is inactive
	}
	tenant, err := u.tenants.TenantBySlug(ctx, claims.Tenant)
	if err != nil || tenant == nil {
		return inactive, nil
	}
	revoked, expired, epoch, blocked, err := u.sessions.SessionState(ctx, tenant.ID, claims.Session)
	if err != nil || revoked || expired || blocked || epoch != claims.Epoch {
		return inactive, nil
	}
	return &IntrospectResult{
		Active:   true,
		Subject:  claims.Subject,
		Session:  claims.Session,
		Tenant:   claims.Tenant,
		Realm:    claims.Realm,
		Roles:    claims.Roles,
		Scopes:   claims.Scopes,
		AMR:      claims.AMR,
		Audience: claims.Audience,
		Expires:  claims.Expires,
		AuthTime: claims.AuthTime,
		IAL:      claims.IAL,
		Epoch:    claims.Epoch,
	}, nil
}

// verify checks signature + time + issuer against the local ring (no
// audience: introspection serves every application).
func (u *introspectInteractor) verify(token string) (*anubis.Claims, error) {
	_, _, footer, err := paseto.Parse(token)
	if err != nil {
		return nil, err
	}
	var tf struct {
		Kid string `json:"kid"`
	}
	if len(footer) > 0 {
		if err := json.Unmarshal(footer, &tf); err != nil {
			return nil, err
		}
	}
	key, err := u.ring.Ring().Lookup(tf.Kid)
	if err != nil || key.Purpose != keyring.PurposeAccess {
		return nil, domain.ErrTokenInvalid
	}
	msg, _, err := paseto.Verify(key.Public, token, nil)
	if err != nil {
		return nil, err
	}
	var claims anubis.Claims
	if err := json.Unmarshal(msg, &claims); err != nil {
		return nil, err
	}
	now := u.clock.Now().Unix()
	if claims.Issuer != u.issuer || (claims.Expires != 0 && now > claims.Expires) ||
		(claims.NotBefore != 0 && now < claims.NotBefore-60) {
		return nil, domain.ErrTokenInvalid
	}
	return &claims, nil
}
