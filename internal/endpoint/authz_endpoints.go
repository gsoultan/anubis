package endpoint

import (
	"context"
	"log/slog"

	"github.com/go-kit/kit/endpoint"

	"github.com/gsoultan/anubis/internal/service"
	"github.com/gsoultan/anubis/internal/usecase"
)

// AuthzEndpoints is the wired endpoint set for AuthzService.
type AuthzEndpoints struct {
	Authorize   endpoint.Endpoint
	Explain     endpoint.Endpoint
	SwitchScope endpoint.Endpoint
}

func NewAuthzEndpoints(svc service.AuthzService, logger *slog.Logger) AuthzEndpoints {
	return AuthzEndpoints{
		Authorize: Chain("authz.authorize", logger)(func(ctx context.Context, req any) (any, error) {
			return svc.Authorize(ctx, req.(usecase.AuthorizeInput))
		}),
		Explain: Chain("authz.explain", logger)(func(ctx context.Context, req any) (any, error) {
			return svc.Explain(ctx, req.(usecase.AuthorizeInput))
		}),
		SwitchScope: Chain("authz.switch_scope", logger)(func(ctx context.Context, req any) (any, error) {
			return svc.SwitchScope(ctx, req.(map[string]string))
		}),
	}
}
