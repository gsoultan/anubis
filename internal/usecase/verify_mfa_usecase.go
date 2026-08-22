package usecase

import "context"

// VerifyMfaUsecase completes a login that answered 202 mfa_required.
type VerifyMfaUsecase interface {
	Execute(ctx context.Context, in VerifyMfaInput) (*TokenPair, error)
}
