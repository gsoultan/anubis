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
	// The list names every session the person has — each application, each
	// device, its IP and last-seen. An application the person signed in to
	// must not read the others with the token it was handed; that is the
	// account holder's own view.
	if !p.FirstParty() {
		return nil, "", apperr.ErrPermissionDenied.
			With("hint", "list sessions with a token Anubis issued for itself")
	}
	list, err := u.sessions.SessionsByIdentity(ctx, p.IdentityID)
	if err != nil {
		return nil, "", apperr.ErrInternal.Wrap(err)
	}
	return list, p.SessionID, nil
}
