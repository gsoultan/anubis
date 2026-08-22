package authapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/clock"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
	"github.com/gsoultan/anubis/pkg/anubis"
	"github.com/gsoultan/anubis/pkg/anubis/paseto"
)

// pasetoTokenIssuer implements TokenIssuer with v4.public access tokens and
// database-backed refresh families.
type pasetoTokenIssuer struct {
	issuer   string
	ring     *keyring.Manager
	apps     tenancyport.ApplicationRepository
	realms   identityport.RealmRepository
	refresh  authport.RefreshRepository
	sessions authport.SessionRepository
	authz    authzport.AuthzRepository
	tenants  tenancyport.TenantRepository
	clock    clock.Clock
}

func NewPasetoTokenIssuer(
	issuer string,
	ring *keyring.Manager,
	apps tenancyport.ApplicationRepository,
	realms identityport.RealmRepository,
	refresh authport.RefreshRepository,
	sessions authport.SessionRepository,
	authz authzport.AuthzRepository,
	tenants tenancyport.TenantRepository,
	clock clock.Clock,
) TokenIssuer {
	return &pasetoTokenIssuer{
		issuer: issuer, ring: ring, apps: apps, realms: realms,
		refresh: refresh, sessions: sessions, authz: authz,
		tenants: tenants, clock: clock,
	}
}

// maxRolesInToken bounds token size; the full set is always available via
// /v1/me and introspection.
const maxRolesInToken = 32

func (t *pasetoTokenIssuer) Issue(ctx context.Context, in IssueInput) (*TokenPair, error) {
	now := t.clock.Now()
	s := in.Session

	accessTTL, refreshTTL, aud, appID, err := t.ttls(ctx, s, in.ClientID)
	if err != nil {
		return nil, err
	}
	_ = appID

	roles, err := t.authz.RolesForIdentity(ctx, s.TenantID, s.IdentityID)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	if len(roles) > maxRolesInToken {
		roles = roles[:maxRolesInToken]
	}

	var scopes map[string]string
	if len(s.ActiveScopes) > 0 {
		if err := json.Unmarshal(s.ActiveScopes, &scopes); err != nil {
			return nil, apperr.ErrInternal.Wrap(err)
		}
	}

	jti, err := secret.New(16)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}

	claims := anubis.Claims{
		Issuer:    t.issuer,
		Subject:   s.IdentityID,
		Audience:  aud,
		Expires:   now.Add(accessTTL).Unix(),
		IssuedAt:  now.Unix(),
		NotBefore: now.Unix(),
		TokenID:   jti,
		Session:   s.ID,
		Tenant:    in.TenantSlug,
		Roles:     roles,
		Scopes:    scopes,
		Realm:     s.RealmCode,
		IAL:       s.AssuranceLevel,
		AMR:       s.AMR,
		AuthTime:  s.AuthTime.Unix(),
		Epoch:     s.TokenEpoch,
		Version:   1,
	}
	body, err := json.Marshal(claims)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}

	key, err := t.ring.Ring().ActiveAccess()
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	footer, _ := json.Marshal(map[string]string{"kid": key.Kid})
	access, err := paseto.Sign(key.Private, body, footer, nil)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}

	pair := &TokenPair{
		AccessToken: access,
		TokenType:   "Bearer",
		ExpiresIn:   int(accessTTL / time.Second),
		SessionID:   s.ID,
	}
	if in.AccessOnly {
		return pair, nil
	}

	// Opaque rotating refresh token: 256-bit random, stored hashed.
	raw, err := secret.New(32)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	refreshToken := "anb_rt_" + raw

	familyID := ""
	generation := 0
	if in.RotateFrom != nil {
		familyID = in.RotateFrom.FamilyID
		generation = in.RotateFrom.Generation + 1
	} else {
		fid, err := newUUIDv7ish(now)
		if err != nil {
			return nil, apperr.ErrInternal.Wrap(err)
		}
		familyID = fid
	}

	newID, err := t.refresh.CreateRefresh(ctx, authdomain.RefreshInput{
		SessionID:  s.ID,
		TenantID:   s.TenantID,
		FamilyID:   familyID,
		Generation: generation,
		TokenHash:  secret.Hash(refreshToken),
		ExpiresAt:  now.Add(refreshTTL),
	})
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	if in.RotateFrom != nil {
		if err := t.refresh.SetRefreshSuccessor(ctx, in.RotateFrom.ID, in.RotateFrom.ExpiresAt, newID); err != nil {
			return nil, apperr.ErrInternal.Wrap(err)
		}
	}
	pair.RefreshToken = refreshToken
	return pair, nil
}

// ttls resolves token lifetimes and audience. Population policy (realm)
// governs; an application with a STRICTER TTL wins. aud defaults to the
// tenant's own console surface when no client is named.
func (t *pasetoTokenIssuer) ttls(ctx context.Context, s *authdomain.SessionView, clientID string) (access, refresh time.Duration, aud []string, appID string, err error) {
	access, refresh = 10*time.Minute, 30*24*time.Hour
	if s.RealmID != "" {
		realm, rerr := t.realms.RealmByID(ctx, s.RealmID)
		if rerr == nil && realm != nil {
			if realm.AccessTokenTTL > 0 {
				access = realm.AccessTokenTTL
			}
			if realm.RefreshTokenTTL > 0 {
				refresh = realm.RefreshTokenTTL
			}
		}
	}
	aud = []string{"anubis"}
	if clientID != "" {
		app, aerr := t.apps.ApplicationBySlug(ctx, s.TenantID, clientID)
		if aerr != nil {
			return 0, 0, nil, "", apperr.ErrInvalidArgument.With("client_id", "unknown application")
		}
		aud = []string{app.Slug}
		appID = app.ID
		if d := time.Duration(app.AccessTokenTTLSecs) * time.Second; d > 0 && d < access {
			access = d
		}
		if d := time.Duration(app.RefreshTokenTTLSecs) * time.Second; d > 0 && d < refresh {
			refresh = d
		}
	}
	return access, refresh, aud, appID, nil
}

// newUUIDv7ish builds a time-ordered 128-bit id for refresh families in the
// uuidv7 layout (the database generates real uuidv7 for rows; families only
// need uniqueness + rough time order).
func newUUIDv7ish(now time.Time) (string, error) {
	raw, err := secret.New(16)
	if err != nil {
		return "", err
	}
	b, _ := base64.RawURLEncoding.DecodeString(raw)
	ms := uint64(now.UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	const hex = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for i, c := range b[:16] {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out = append(out, '-')
		}
		out = append(out, hex[c>>4], hex[c&0xf])
	}
	return string(out), nil
}
