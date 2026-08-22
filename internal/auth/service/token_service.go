package authsvc

import (
	"context"

	tokenapp "github.com/gsoultan/anubis/internal/auth/app/token"
)

// TokenService is the introspection/revocation surface (proto TokenService).
type TokenService interface {
	Introspect(ctx context.Context, token string) (*tokenapp.IntrospectResult, error)
	Revoke(ctx context.Context, token, hint string) error
}
