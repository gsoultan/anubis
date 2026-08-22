package usecase

import "context"

// LoginUsecase is password sign-in. Failure responses are uniform in message
// AND timing (docs/security.md): the KDF runs even when the user does not
// exist.
type LoginUsecase interface {
	Execute(ctx context.Context, in LoginInput) (*LoginOutput, error)
}
