package apiconnect

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"connectrpc.com/connect"

	identityport "github.com/gsoultan/anubis/internal/identity/port"
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
	creds   identityport.CredentialRepository
	clock   clock.Clock
}

func NewAuthnInterceptor(issuer string, ring *keyring.Manager, tenants tenancyport.TenantRepository, creds identityport.CredentialRepository, clock clock.Clock) *AuthnInterceptor {
	return &AuthnInterceptor{issuer: issuer, ring: ring, tenants: tenants, creds: creds, clock: clock}
}

func (i *AuthnInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		auth := req.Header().Get("Authorization")
		const bearer = "Bearer "
		if len(auth) > len(bearer) && strings.EqualFold(auth[:len(bearer)], bearer) {
			cred := auth[len(bearer):]
			switch {
			case strings.HasPrefix(cred, "v4.public."):
				if p := i.principalFromToken(ctx, cred); p != nil {
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

func (i *AuthnInterceptor) principalFromToken(ctx context.Context, token string) *authctx.Principal {
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
	cred, err := i.creds.CredentialByLookup(ctx, lookup)
	if err != nil || cred == nil || cred.Blocked || cred.IdentityStatus != "active" {
		return nil
	}
	if cred.ExpiresAt != nil && !cred.ExpiresAt.After(i.clock.Now()) {
		return nil
	}
	if !secret.Equal(secret.Hash(secretPart), mustHexOrRaw(cred.SecretHash)) {
		return nil
	}
	i.creds.TouchCredentialUsed(ctx, cred.ID, 0)
	tenant, err := i.tenants.TenantByID(ctx, cred.TenantID)
	if err != nil || tenant == nil {
		return nil
	}
	return &authctx.Principal{
		IdentityID: cred.IdentityID,
		TenantID:   cred.TenantID,
		TenantSlug: tenant.Slug,
		Service:    true,
		Epoch:      cred.TokenEpoch,
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
