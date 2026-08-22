package service

import (
	"context"

	"github.com/gsoultan/anubis/internal/usecase"
)

// TokenService is the introspection/revocation surface (proto TokenService).
type TokenService interface {
	Introspect(ctx context.Context, token string) (*usecase.IntrospectResult, error)
	Revoke(ctx context.Context, token, hint string) error
}
