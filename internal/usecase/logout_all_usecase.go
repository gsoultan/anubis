package usecase

import "context"

// LogoutAllUsecase revokes every session of the calling identity, bumps
// token_epoch (the global kill switch) and fires back-channel logout to every
// application that registered for it.
type LogoutAllUsecase interface {
	Execute(ctx context.Context) (revoked int, err error)
}
