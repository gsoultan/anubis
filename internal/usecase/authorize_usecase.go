package usecase

import "context"

// AuthorizeUsecase is the decision endpoint. The verdict comes from the SQL
// engine; step-up (amr/auth_time vs the permission's declared requirements)
// composes on top.
type AuthorizeUsecase interface {
	Execute(ctx context.Context, in AuthorizeInput) (*Decision, error)
}
