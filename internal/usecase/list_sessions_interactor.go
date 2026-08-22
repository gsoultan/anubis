package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

// listSessionsInteractor implements ListSessionsUsecase.
type listSessionsInteractor struct {
	sessions repository.SessionRepository
}

func NewListSessionsInteractor(sessions repository.SessionRepository) ListSessionsUsecase {
	return &listSessionsInteractor{sessions: sessions}
}

func (u *listSessionsInteractor) Execute(ctx context.Context) ([]repository.SessionInfo, string, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, "", domain.ErrUnauthenticated
	}
	list, err := u.sessions.SessionsByIdentity(ctx, p.IdentityID)
	if err != nil {
		return nil, "", domain.ErrInternal.Wrap(err)
	}
	return list, p.SessionID, nil
}
