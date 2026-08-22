package service

import (
	"context"

	"github.com/gsoultan/anubis/internal/repository"
	"github.com/gsoultan/anubis/internal/usecase"
)

type sessionService struct {
	me            usecase.GetMeUsecase
	list          usecase.ListSessionsUsecase
	logoutSession usecase.LogoutSessionUsecase
}

func NewSessionService(me usecase.GetMeUsecase, list usecase.ListSessionsUsecase, logoutSession usecase.LogoutSessionUsecase) SessionService {
	return &sessionService{me: me, list: list, logoutSession: logoutSession}
}

func (s *sessionService) GetMe(ctx context.Context) (*usecase.Me, error) {
	return s.me.Execute(ctx)
}

func (s *sessionService) ListSessions(ctx context.Context) ([]repository.SessionInfo, string, error) {
	return s.list.Execute(ctx)
}

func (s *sessionService) RevokeSession(ctx context.Context, sessionID string) error {
	return s.logoutSession.Execute(ctx, sessionID)
}
