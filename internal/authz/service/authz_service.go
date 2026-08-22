package authzsvc

import (
	"context"

	authapp "github.com/gsoultan/anubis/internal/auth/app"
	authzapp "github.com/gsoultan/anubis/internal/authz/app"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
)

// AuthzService is the decision surface (proto AuthzService).
type AuthzService interface {
	Authorize(ctx context.Context, in authzapp.AuthorizeInput) (*authzdomain.Decision, error)
	Explain(ctx context.Context, in authzapp.AuthorizeInput) (*authzdomain.Explanation, error)
	SwitchScope(ctx context.Context, scopes map[string]string) (*authapp.TokenPair, error)
}
