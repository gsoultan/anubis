package accesslink

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authapp "github.com/gsoultan/anubis/internal/auth/app"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/validate"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
	"github.com/gsoultan/anubis/pkg/anubis"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

// LinkPrefix is what a link token starts with, so one pasted into the wrong
// field is recognisable, and so a client can refuse to send anything else.
const LinkPrefix = "anb_lk_"

// linkAccessTTL is deliberately short: the link is the durable thing, the
// token is what one page load holds.
const linkAccessTTL = 10 * time.Minute

type openInteractor struct {
	issuer  string
	ring    *keyring.Manager
	shares  authport.LinkedShareRepository
	tenants tenancyport.TenantRepository
	apps    tenancyport.ApplicationRepository
	ids     identityport.IdentityRepository
	clock   clock.Clock
	audit   auditport.Auditor
}

func NewOpenInteractor(
	issuer string,
	ring *keyring.Manager,
	shares authport.LinkedShareRepository,
	tenants tenancyport.TenantRepository,
	apps tenancyport.ApplicationRepository,
	ids identityport.IdentityRepository,
	clk clock.Clock,
	audit auditport.Auditor,
) OpenUsecase {
	return &openInteractor{
		issuer: issuer, ring: ring, shares: shares, tenants: tenants, apps: apps,
		ids: ids, clock: clk, audit: audit,
	}
}

func (u *openInteractor) OpenAccessLink(ctx context.Context, in OpenInput) (*authapp.TokenPair, error) {
	// Every refusal is the same refusal: a link that is wrong, lapsed,
	// revoked, for another tenant or for a recipient who cannot sign in all
	// say "invalid link", and the audit log says which.
	if !strings.HasPrefix(in.Token, LinkPrefix) || len(in.Token) > 128 ||
		!validate.ValidSlug(in.Tenant) || !validate.ValidSlug(in.ClientID) {
		return nil, apperr.ErrTokenInvalid
	}
	tenant, err := u.tenants.TenantBySlug(ctx, in.Tenant)
	if err != nil || tenant == nil {
		return nil, apperr.ErrTokenInvalid
	}
	sh, err := u.shares.ShareByLinkHash(ctx, secret.Hash(in.Token))
	if err != nil || sh == nil || sh.TenantID != tenant.ID {
		u.deny(ctx, tenant.ID, "", "no_such_link")
		return nil, apperr.ErrTokenInvalid
	}
	identity, err := u.ids.Identity(ctx, tenant.ID, sh.ToIdentityID)
	if err != nil || identity == nil {
		u.deny(ctx, tenant.ID, sh.ToIdentityID, "recipient_missing")
		return nil, apperr.ErrTokenInvalid
	}
	if aerr := identity.CanAuthenticate(); aerr != nil {
		u.deny(ctx, tenant.ID, identity.ID, apperr.AsError(aerr).Code)
		return nil, apperr.ErrTokenInvalid
	}
	app, err := u.apps.ApplicationBySlug(ctx, tenant.ID, in.ClientID)
	if err != nil || app == nil || app.Status != "active" {
		u.deny(ctx, tenant.ID, identity.ID, "unknown_application")
		return nil, apperr.ErrTokenInvalid
	}
	// A record lives in a tenant's application, never in Anubis's own: a
	// token for "anubis" or "console" is the audience self-service trusts.
	if app.IsSystem {
		u.deny(ctx, tenant.ID, identity.ID, "system_application")
		return nil, apperr.ErrTokenInvalid
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
	ttl := linkAccessTTL
	if d := time.Duration(app.AccessTokenTTLSecs) * time.Second; d > 0 && d < ttl {
		ttl = d
	}
	// Bound to the share's end as well: a token must not outlive the link
	// it came from.
	if sh.ValidUntil.Before(now.Add(ttl)) {
		ttl = sh.ValidUntil.Sub(now)
	}
	body, err := json.Marshal(anubis.Claims{
		Issuer: u.issuer, Subject: identity.ID, Audience: []string{app.Slug},
		Expires: now.Add(ttl).Unix(), IssuedAt: now.Unix(), NotBefore: now.Unix(),
		TokenID: jti, Tenant: tenant.Slug, Realm: identity.RealmCode,
		AMR: []string{"link"}, AuthTime: now.Unix(),
		Scopes: map[string]string{"_record": sh.Record},
		IAL:    identity.AssuranceLevel, Epoch: identity.TokenEpoch, Version: 1,
	})
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	footer, _ := json.Marshal(map[string]string{"kid": key.Kid})
	token, err := paseto.Sign(key.Private, body, footer, nil)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	u.shares.TouchShareLink(ctx, sh.ID)
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: tenant.ID, ActorID: identity.ID, ActorKind: "identity",
		TargetID: sh.ID, Action: "auth.link_open", Result: "allow",
		IP:     authctx.ClientIP(ctx),
		Detail: jsonx.Must(map[string]string{"record": sh.Record, "client_id": app.Slug, "from": sh.FromIdentityID}),
	})
	return &authapp.TokenPair{
		AccessToken: token, TokenType: "Bearer", ExpiresIn: int(ttl / time.Second),
	}, nil
}

func (u *openInteractor) deny(ctx context.Context, tenantID, identityID, reason string) {
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: tenantID, ActorID: identityID, ActorKind: "identity",
		Action: "auth.link_open", Result: "deny",
		IP: authctx.ClientIP(ctx), Detail: jsonx.Must(map[string]string{"reason": reason}),
	})
}
