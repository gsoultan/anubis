package authsvc

import (
	"context"

	sessionapp "github.com/gsoultan/anubis/internal/auth/app/session"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
)

// SessionService is the signed-in user's own view (proto SessionService).
type SessionService interface {
	GetMe(ctx context.Context) (*sessionapp.Me, error)
	ListSessions(ctx context.Context) (sessions []authdomain.SessionInfo, currentID string, err error)
	RevokeSession(ctx context.Context, sessionID string) error
}
