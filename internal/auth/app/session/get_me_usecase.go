package sessionapp

import "context"

// GetMeUsecase is the signed-in user's own view: profile, roles, effective
// permissions, active scope.
type GetMeUsecase interface {
	Execute(ctx context.Context) (*Me, error)
}
