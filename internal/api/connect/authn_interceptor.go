package apiconnect

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"connectrpc.com/connect"

	authport "github.com/gsoultan/anubis/internal/auth/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
	"github.com/gsoultan/anubis/pkg/anubis"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

// AuthnInterceptor authenticates whoever presents credentials — bearer
// access token (offline verify against the ring) or anb_live API key (one
// index probe) — and attaches the principal. It ENFORCES nothing: usecases
// demand principals where they need them, so a public RPC costs no policy
// table here.
type AuthnInterceptor struct {
	issuer  string
	ring    *keyring.Manager
	tenants tenancyport.TenantRepository
	apiKeys authport.APIKeyRepository
	clock   clock.Clock
}

func NewAuthnInterceptor(issuer string, ring *keyring.Manager, tenants tenancyport.TenantRepository, apiKeys authport.APIKeyRepository, clock clock.Clock) *AuthnInterceptor {
	return &AuthnInterceptor{issuer: issuer, ring: ring, tenants: tenants, apiKeys: apiKeys, clock: clock}
}

func (i *AuthnInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		auth := req.Header().Get("Authorization")
		// Which tenant an operator is working in. It is only ever a request
		// for one — the guard decides whether they may.
		wanted := req.Header().Get(TenantHeader)
		const bearer = "Bearer "
		if len(auth) > len(bearer) && strings.EqualFold(auth[:len(bearer)], bearer) {
			cred := auth[len(bearer):]
			switch {
			case strings.HasPrefix(cred, "v4.public."):
				if p := i.principalFromToken(ctx, cred, wanted); p != nil {
					ctx = authctx.With(ctx, p)
				}
			case strings.HasPrefix(cred, "anb_live_"):
				if p := i.principalFromAPIKey(ctx, cred); p != nil {
					ctx = authctx.With(ctx, p)
				}
			}
		}
		return next(ctx, req)
	}
}

func (i *AuthnInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *AuthnInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

// platformAudience mirrors controlapp.PlatformAudience. It is repeated rather
// than imported because this transport package must not depend on a bounded
// context; the pair is pinned by a test.
const platformAudience = "anubis-platform"

// TenantHeader names the tenant a platform operator is administering. It is
// meaningless on a tenant identity's token, whose tenant is already fixed by
// the token itself and cannot be asked to change.
const TenantHeader = "X-Anubis-Tenant"

func (i *AuthnInterceptor) principalFromToken(ctx context.Context, token, wantTenant string) *authctx.Principal {
	_, _, footer, err := paseto.Parse(token)
	if err != nil {
		return nil
	}
	var tf struct {
		Kid string `json:"kid"`
	}
	if len(footer) > 0 && json.Unmarshal(footer, &tf) != nil {
		return nil
	}
	key, err := i.ring.Ring().Lookup(tf.Kid)
	if err != nil || key.Purpose != keyring.PurposeAccess {
		return nil
	}
	msg, _, err := paseto.Verify(key.Public, token, nil)
	if err != nil {
		return nil
	}
	var claims anubis.Claims
	if json.Unmarshal(msg, &claims) != nil {
		return nil
	}
	now := i.clock.Now().Unix()
	if claims.Issuer != i.issuer ||
		(claims.Expires != 0 && now > claims.Expires) ||
		(claims.NotBefore != 0 && now < claims.NotBefore-60) {
		return nil
	}
	// A platform token carries no tenant, because an operator belongs to
	// none (ADR-0011). The audience is what separates the two: a tenant's
	// verifier must never accept one of these, and this must never mistake a
	// tenant token for an operator's.
	if claims.Tenant == "" {
		for _, aud := range claims.Audience {
			if aud != platformAudience {
				continue
			}
			p := &authctx.Principal{
				IdentityID: claims.Subject,
				Roles:      claims.Roles,
				Epoch:      claims.Epoch,
				Platform:   true,
				Token:      token,
			}
			// The operator asked to work in a tenant. Resolving it here only
			// records WHICH tenant; whether they may is the guard's call,
			// against their assignments, on every request — so a revoked
			// assignment stops working immediately rather than when a token
			// happens to expire.
			if wantTenant != "" {
				if t, terr := i.tenants.TenantBySlug(ctx, wantTenant); terr == nil && t != nil {
					p.TenantID, p.TenantSlug = t.ID, t.Slug
				}
			}
			return p
		}
		return nil
	}

	tenant, err := i.tenants.TenantBySlug(ctx, claims.Tenant)
	if err != nil || tenant == nil {
		return nil
	}

	return &authctx.Principal{
		IdentityID: claims.Subject,
		TenantID:   tenant.ID,
		TenantSlug: claims.Tenant,
		SessionID:  claims.Session,
		Realm:      claims.Realm,
		Roles:      claims.Roles,
		Scopes:     claims.Scopes,
		AMR:        claims.AMR,
		AuthTime:   time.Unix(claims.AuthTime, 0),
		IAL:        claims.IAL,
		Epoch:      claims.Epoch,
		Audience:   claims.Audience,
		Token:      token,
	}
}

func (i *AuthnInterceptor) principalFromAPIKey(ctx context.Context, key string) *authctx.Principal {
	lookup, secretPart, ok := secret.SplitAPIKey(key)
	if !ok {
		return nil
	}
	// The tenant's machine credential (migration 0030): it authenticates as
	// the tenant's SYSTEM, never as any person, so there is no identity to
	// resolve and no identity status to consult. The tenant's own status
	// stands in for it — a suspended tenant's keys stop with it.
	k, err := i.apiKeys.APIKeyByLookup(ctx, lookup)
	if err != nil || k == nil || k.TenantStatus != "active" {
		return nil
	}
	if k.ExpiresAt != nil && !k.ExpiresAt.After(i.clock.Now()) {
		return nil
	}
	if !secret.Equal(secret.Hash(secretPart), mustHexOrRaw(k.SecretHash)) {
		return nil
	}
	i.apiKeys.TouchAPIKeyUsed(ctx, k.ID)
	return &authctx.Principal{
		// The key's own id is the subject, so audit names WHICH credential
		// acted without pretending a person did.
		IdentityID: k.ID,
		TenantID:   k.TenantID,
		TenantSlug: k.TenantSlug,
		Service:    true,
	}
}

// mustHexOrRaw accepts the stored sha256 either hex-encoded or raw base64;
// credentials store hex per 0002's comment.
func mustHexOrRaw(s string) []byte {
	if b, err := hexDecode(s); err == nil {
		return b
	}
	return []byte(s)
}

func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, errOddHex
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi, ok1 := hexNibble(s[i])
		lo, ok2 := hexNibble(s[i+1])
		if !ok1 || !ok2 {
			return nil, errOddHex
		}
		out[i/2] = hi<<4 | lo
	}
	return out, nil
}

func hexNibble(c byte) (byte, bool) {
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

var errOddHex = errInvalidHex{}

type errInvalidHex struct{}

func (errInvalidHex) Error() string { return "invalid hex" }
