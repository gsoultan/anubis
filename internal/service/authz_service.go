package service

import (
	"context"

	"github.com/gsoultan/anubis/internal/usecase"
)

// AuthzService is the decision surface (proto AuthzService).
type AuthzService interface {
	Authorize(ctx context.Context, in usecase.AuthorizeInput) (*usecase.Decision, error)
	Explain(ctx context.Context, in usecase.AuthorizeInput) (*usecase.Explanation, error)
	SwitchScope(ctx context.Context, scopes map[string]string) (*usecase.TokenPair, error)
}
