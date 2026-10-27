package authzep

import (
	"context"
	"log/slog"

	"github.com/go-kit/kit/endpoint"
	authzapp "github.com/gsoultan/anubis/internal/authz/app"
	authzreach "github.com/gsoultan/anubis/internal/authz/app/reach"
	authzsvc "github.com/gsoultan/anubis/internal/authz/service"
	"github.com/gsoultan/anubis/internal/platform/mw"
)

// AuthzEndpoints is the wired endpoint set for AuthzService.
type AuthzEndpoints struct {
	Authorize    endpoint.Endpoint
	Explain      endpoint.Endpoint
	SwitchScope  endpoint.Endpoint
	GrantedNodes endpoint.Endpoint
}

func NewAuthzEndpoints(svc authzsvc.AuthzService, logger *slog.Logger) AuthzEndpoints {
	return AuthzEndpoints{
		Authorize: mw.Chain("authz.authorize", logger)(func(ctx context.Context, req any) (any, error) {
			return svc.Authorize(ctx, req.(authzapp.AuthorizeInput))
		}),
		Explain: mw.Chain("authz.explain", logger)(func(ctx context.Context, req any) (any, error) {
			return svc.Explain(ctx, req.(authzapp.AuthorizeInput))
		}),
		SwitchScope: mw.Chain("authz.switch_scope", logger)(func(ctx context.Context, req any) (any, error) {
			return svc.SwitchScope(ctx, req.(map[string]string))
		}),
		GrantedNodes: mw.Chain("authz.granted_nodes", logger)(func(ctx context.Context, req any) (any, error) {
			return svc.ListGrantedNodes(ctx, req.(authzreach.NodesInput))
		}),
	}
}
