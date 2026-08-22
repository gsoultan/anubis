package service

import (
	"context"

	"github.com/gsoultan/anubis/internal/repository"
	"github.com/gsoultan/anubis/internal/usecase"
)

// SessionService is the signed-in user's own view (proto SessionService).
type SessionService interface {
	GetMe(ctx context.Context) (*usecase.Me, error)
	ListSessions(ctx context.Context) (sessions []repository.SessionInfo, currentID string, err error)
	RevokeSession(ctx context.Context, sessionID string) error
}
