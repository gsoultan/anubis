package usecase

import "context"

// RegisterUsecase is self-registration — public realms with
// self_registration=true ONLY. A public registration endpoint hands an
// unauthenticated attacker an authenticated account; everything here is
// rate-limited upstream and consent-recorded.
type RegisterUsecase interface {
	Execute(ctx context.Context, in RegisterInput) (*RegisterOutput, error)
}
