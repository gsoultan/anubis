package tokenapp

import (
	"context"
	"encoding/json"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
	"github.com/gsoultan/anubis/pkg/anubis"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

// clientAccessTTL is short because there is nothing to revoke: a client
// token carries no session id, so expiry IS the revocation mechanism.
const clientAccessTTL = 10 * time.Minute

type clientCredentialsInteractor struct {
	issuer  string
	ring    *keyring.Manager
	apps    tenancyport.ApplicationRepository
	tenants tenancyport.TenantRepository
	clock   clock.Clock
	audit   auditport.Auditor
}

func NewClientCredentialsInteractor(
	issuer string,
	ring *keyring.Manager,
	apps tenancyport.ApplicationRepository,
	tenants tenancyport.TenantRepository,
	clk clock.Clock,
	audit auditport.Auditor,
) ClientCredentialsUsecase {
	return &clientCredentialsInteractor{
		issuer: issuer, ring: ring, apps: apps, tenants: tenants, clock: clk, audit: audit,
	}
}

func (u *clientCredentialsInteractor) Execute(ctx context.Context, in ClientCredentialsInput) (*ClientCredentialsOutput, error) {
	tenant, err := u.tenants.TenantBySlug(ctx, in.Tenant)
	if err != nil || tenant == nil {
		return nil, apperr.ErrInvalidCredentials
	}
	app, err := u.apps.ApplicationBySlug(ctx, tenant.ID, in.ClientID)
	if err != nil || app == nil || app.Status != "active" || app.ClientSecretHash == "" {
		// Same answer for unknown client and wrong secret — a client id is
		// not supposed to be enumerable either.
		return nil, apperr.ErrInvalidCredentials
	}
	// Constant-time compare of the stored hash; never ==.
	if !secret.Equal(secret.Hash(in.ClientSecret), decodeHex(app.ClientSecretHash)) {
		u.audit.Emit(ctx, auditdomain.AuditEvent{
			TenantID: tenant.ID, ActorKind: "service", TargetID: app.ID,
			Action: "auth.client_credentials", Result: "deny",
			IP: authctx.ClientIP(ctx), Detail: jsonx.Must(map[string]string{"client_id": in.ClientID}),
		})
		return nil, apperr.ErrInvalidCredentials
	}

	audience := in.Audience
	if audience == "" {
		audience = app.Slug
	}
	key, err := u.ring.Ring().ActiveAccess()
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	jti, err := secret.New(16)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	now := u.clock.Now()
	ttl := clientAccessTTL
	if d := time.Duration(app.AccessTokenTTLSecs) * time.Second; d > 0 && d < ttl {
		ttl = d
	}
	// sub is the APPLICATION, and there is no sid: consumers can tell a
	// service token from a user token without a special claim.
	body, err := json.Marshal(anubis.Claims{
		Issuer: u.issuer, Subject: "app_" + app.Slug, Audience: []string{audience},
		Expires: now.Add(ttl).Unix(), IssuedAt: now.Unix(), NotBefore: now.Unix(),
		TokenID: jti, Tenant: tenant.Slug, AMR: []string{"client_secret"},
		AuthTime: now.Unix(), Epoch: 1, Version: 1,
	})
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	footer, _ := json.Marshal(map[string]string{"kid": key.Kid})
	token, err := paseto.Sign(key.Private, body, footer, nil)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: tenant.ID, ActorKind: "service", TargetID: app.ID,
		Action: "auth.client_credentials", Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: jsonx.Must(map[string]string{"aud": audience}),
	})
	return &ClientCredentialsOutput{
		AccessToken: token, TokenType: "Bearer", ExpiresIn: int(ttl / time.Second),
	}, nil
}

func decodeHex(s string) []byte {
	if len(s)%2 != 0 {
		return nil
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi, ok1 := hexVal(s[i])
		lo, ok2 := hexVal(s[i+1])
		if !ok1 || !ok2 {
			return nil
		}
		out[i/2] = hi<<4 | lo
	}
	return out
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
