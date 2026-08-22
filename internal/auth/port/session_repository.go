package authport

import (
	"context"
	"time"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
)

// SessionRepository is the session anchor: sign-out revokes here and
// everything else follows.
type SessionRepository interface {
	CreateSession(ctx context.Context, in authdomain.SessionInput) (*authdomain.Session, error)
	SessionLive(ctx context.Context, id string) (*authdomain.SessionView, error)
	SessionsByIdentity(ctx context.Context, identityID string) ([]authdomain.SessionInfo, error)
	RevokeSession(ctx context.Context, tenantID, id, reason string) (*authdomain.RevokedSession, error)
	RevokeAllSessions(ctx context.Context, tenantID, identityID, reason string) ([]authdomain.RevokedSession, error)
	TouchSession(ctx context.Context, id string)
	UpdateSessionScopes(ctx context.Context, id string, scopes []byte) error
	UpgradeSessionAMR(ctx context.Context, id string, amr []string) (time.Time, error)
	SetSessionCookieHash(ctx context.Context, id string, hash []byte) error
	SessionByCookieHash(ctx context.Context, hash []byte) (*authdomain.SessionView, error)
	SessionState(ctx context.Context, tenantID, id string) (revoked bool, expired bool, epoch int, blocked bool, err error)
}
