package usecase

import "context"

// RefreshUsecase rotates a refresh token. Presenting an already-consumed
// token is treated as THEFT: the whole family and its session are revoked,
// and the event must page a human (docs/api.md).
type RefreshUsecase interface {
	Execute(ctx context.Context, in RefreshInput) (*TokenPair, error)
}
