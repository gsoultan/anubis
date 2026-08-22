package sessionapp

import "context"

// LogoutUsecase revokes the CALLING session (this device).
type LogoutUsecase interface {
	Execute(ctx context.Context) error
}
