package mfa

import (
	"context"

	authapp "github.com/gsoultan/anubis/internal/auth/app"
)

// VerifyMfaUsecase completes a login that answered 202 mfa_required.
type VerifyMfaUsecase interface {
	Execute(ctx context.Context, in VerifyMfaInput) (*authapp.TokenPair, error)
}
