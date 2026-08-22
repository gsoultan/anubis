package authep

import (
	"context"
	"log/slog"

	"github.com/go-kit/kit/endpoint"
	authsvc "github.com/gsoultan/anubis/internal/auth/service"
	"github.com/gsoultan/anubis/internal/platform/mw"
)

// TokenEndpoints is the wired endpoint set for TokenService.
type TokenEndpoints struct {
	Introspect endpoint.Endpoint
	Revoke     endpoint.Endpoint
}

// RevokeRequest crosses the endpoint boundary for Revoke.
type RevokeRequest struct {
	Token string
	Hint  string
}

func NewTokenEndpoints(svc authsvc.TokenService, logger *slog.Logger) TokenEndpoints {
	return TokenEndpoints{
		Introspect: mw.Chain("token.introspect", logger)(func(ctx context.Context, req any) (any, error) {
			return svc.Introspect(ctx, req.(string))
		}),
		Revoke: mw.Chain("token.revoke", logger)(func(ctx context.Context, req any) (any, error) {
			r := req.(RevokeRequest)
			return nil, svc.Revoke(ctx, r.Token, r.Hint)
		}),
	}
}
