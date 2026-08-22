package usecase

import "context"

// LogoutSessionUsecase revokes one named session of the calling identity.
type LogoutSessionUsecase interface {
	Execute(ctx context.Context, sessionID string) error
}
