package endpoint

import (
	"context"
	"log/slog"

	"github.com/go-kit/kit/endpoint"

	"github.com/gsoultan/anubis/internal/service"
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

func NewTokenEndpoints(svc service.TokenService, logger *slog.Logger) TokenEndpoints {
	return TokenEndpoints{
		Introspect: Chain("token.introspect", logger)(func(ctx context.Context, req any) (any, error) {
			return svc.Introspect(ctx, req.(string))
		}),
		Revoke: Chain("token.revoke", logger)(func(ctx context.Context, req any) (any, error) {
			r := req.(RevokeRequest)
			return nil, svc.Revoke(ctx, r.Token, r.Hint)
		}),
	}
}
