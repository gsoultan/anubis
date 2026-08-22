package authep

import (
	"context"
	"log/slog"

	"github.com/go-kit/kit/endpoint"
	authsvc "github.com/gsoultan/anubis/internal/auth/service"
	"github.com/gsoultan/anubis/internal/platform/mw"
)

// SessionEndpoints is the wired endpoint set for SessionService.
type SessionEndpoints struct {
	GetMe         endpoint.Endpoint
	ListSessions  endpoint.Endpoint
	RevokeSession endpoint.Endpoint
}

// SessionList pairs the device list with the calling session id.
type SessionList struct {
	Sessions  any
	CurrentID string
}

func NewSessionEndpoints(svc authsvc.SessionService, logger *slog.Logger) SessionEndpoints {
	return SessionEndpoints{
		GetMe: mw.Chain("session.me", logger)(func(ctx context.Context, _ any) (any, error) {
			return svc.GetMe(ctx)
		}),
		ListSessions: mw.Chain("session.list", logger)(func(ctx context.Context, _ any) (any, error) {
			list, current, err := svc.ListSessions(ctx)
			if err != nil {
				return nil, err
			}
			return SessionList{Sessions: list, CurrentID: current}, nil
		}),
		RevokeSession: mw.Chain("session.revoke", logger)(func(ctx context.Context, req any) (any, error) {
			return nil, svc.RevokeSession(ctx, req.(string))
		}),
	}
}
