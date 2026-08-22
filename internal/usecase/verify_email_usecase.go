package usecase

import "context"

// VerifyEmailUsecase consumes an email verification token and activates the
// pending identity.
type VerifyEmailUsecase interface {
	Execute(ctx context.Context, token string) error
}
