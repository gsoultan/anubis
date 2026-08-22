package sessionapp

import (
	"context"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// listSessionsInteractor implements ListSessionsUsecase.
type listSessionsInteractor struct {
	sessions authport.SessionRepository
}

func NewListSessionsInteractor(sessions authport.SessionRepository) ListSessionsUsecase {
	return &listSessionsInteractor{sessions: sessions}
}

func (u *listSessionsInteractor) Execute(ctx context.Context) ([]authdomain.SessionInfo, string, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, "", apperr.ErrUnauthenticated
	}
	list, err := u.sessions.SessionsByIdentity(ctx, p.IdentityID)
	if err != nil {
		return nil, "", apperr.ErrInternal.Wrap(err)
	}
	return list, p.SessionID, nil
}
