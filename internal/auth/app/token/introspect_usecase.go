package tokenapp

import "context"

// IntrospectUsecase answers "is this token good RIGHT NOW" — the online
// check that sees revocation and epoch bumps the offline verifier cannot.
type IntrospectUsecase interface {
	Execute(ctx context.Context, token string) (*IntrospectResult, error)
}
