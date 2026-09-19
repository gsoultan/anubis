package tokenapp

import (
	"context"
	"encoding/json"

	authport "github.com/gsoultan/anubis/internal/auth/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/accesstoken"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/clock"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
	"github.com/gsoultan/anubis/pkg/anubis"
)

// introspectInteractor implements IntrospectUsecase.
type introspectInteractor struct {
	issuer   string
	ring     *keyring.Manager
	sessions authport.SessionRepository
	tenants  tenancyport.TenantRepository
	clock    clock.Clock
}

func NewIntrospectInteractor(
	issuer string,
	ring *keyring.Manager,
	sessions authport.SessionRepository,
	tenants tenancyport.TenantRepository,
	clock clock.Clock,
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
	kid, err := accesstoken.Kid(token)
	if err != nil {
		return nil, err
	}
	key, err := u.ring.Ring().Lookup(kid)
	if err != nil || key.Purpose != keyring.PurposeAccess {
		return nil, apperr.ErrTokenInvalid
	}
	msg, err := accesstoken.Verify(key.Public, token)
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
		return nil, apperr.ErrTokenInvalid
	}
	return &claims, nil
}
