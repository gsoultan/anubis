package usecase

import "context"

// RevokeUsecase implements RFC 7009-shaped revocation: whoever bears a token
// may kill it. Refresh tokens revoke their whole family; access tokens
// revoke their session.
type RevokeUsecase interface {
	Execute(ctx context.Context, token, hint string) error
}
