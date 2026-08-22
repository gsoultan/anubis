package authsvc

import (
	"context"

	sessionapp "github.com/gsoultan/anubis/internal/auth/app/session"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
)

type sessionService struct {
	me            sessionapp.GetMeUsecase
	list          sessionapp.ListSessionsUsecase
	logoutSession sessionapp.LogoutSessionUsecase
}

func NewSessionService(me sessionapp.GetMeUsecase, list sessionapp.ListSessionsUsecase, logoutSession sessionapp.LogoutSessionUsecase) SessionService {
	return &sessionService{me: me, list: list, logoutSession: logoutSession}
}

func (s *sessionService) GetMe(ctx context.Context) (*sessionapp.Me, error) {
	return s.me.Execute(ctx)
}

func (s *sessionService) ListSessions(ctx context.Context) ([]authdomain.SessionInfo, string, error) {
	return s.list.Execute(ctx)
}

func (s *sessionService) RevokeSession(ctx context.Context, sessionID string) error {
	return s.logoutSession.Execute(ctx, sessionID)
}
