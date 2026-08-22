package repository

import (
	"context"
	"github.com/gsoultan/anubis/internal/domain"
	"time"
)

// SessionRepository is the session anchor: sign-out revokes here and
// everything else follows.
type SessionRepository interface {
	CreateSession(ctx context.Context, in SessionInput) (*domain.Session, error)
	SessionLive(ctx context.Context, id string) (*SessionView, error)
	SessionsByIdentity(ctx context.Context, identityID string) ([]SessionInfo, error)
	RevokeSession(ctx context.Context, tenantID, id, reason string) (*RevokedSession, error)
	RevokeAllSessions(ctx context.Context, tenantID, identityID, reason string) ([]RevokedSession, error)
	TouchSession(ctx context.Context, id string)
	UpdateSessionScopes(ctx context.Context, id string, scopes []byte) error
	UpgradeSessionAMR(ctx context.Context, id string, amr []string) (time.Time, error)
	SetSessionCookieHash(ctx context.Context, id string, hash []byte) error
	SessionByCookieHash(ctx context.Context, hash []byte) (*SessionView, error)
	SessionState(ctx context.Context, tenantID, id string) (revoked bool, expired bool, epoch int, blocked bool, err error)
}
