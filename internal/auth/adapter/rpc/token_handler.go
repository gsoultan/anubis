package authrpc

import (
	"context"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	"github.com/gsoultan/anubis/internal/auth/app/token"
	authep "github.com/gsoultan/anubis/internal/auth/endpoint"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// TokenHandler implements anubisv1connect.TokenServiceHandler.
type TokenHandler struct {
	eps authep.TokenEndpoints
}

func NewTokenHandler(eps authep.TokenEndpoints) *TokenHandler { return &TokenHandler{eps: eps} }

var _ anubisv1connect.TokenServiceHandler = (*TokenHandler)(nil)

// Introspect is service-auth only (api.md): most applications should verify
// offline; whoever calls this is trusted with session-state answers.
func (h *TokenHandler) Introspect(ctx context.Context, req *connect.Request[anubisv1.IntrospectRequest]) (*connect.Response[anubisv1.IntrospectResponse], error) {
	if p, ok := authctx.From(ctx); !ok || !p.Service {
		return nil, apiconnect.Err(ctx, apperr.ErrUnauthenticated)
	}
	out, err := h.eps.Introspect(ctx, req.Msg.Token)
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	r := out.(*tokenapp.IntrospectResult)
	return connect.NewResponse(&anubisv1.IntrospectResponse{
		Active: r.Active, Sub: r.Subject, Sid: r.Session, Tid: r.Tenant,
		Realm: r.Realm, Roles: r.Roles, Scopes: r.Scopes, Amr: r.AMR,
		Aud: r.Audience, Exp: r.Expires, AuthTime: r.AuthTime,
		Ial: int32(r.IAL), Epoch: int32(r.Epoch),
	}), nil
}

func (h *TokenHandler) Revoke(ctx context.Context, req *connect.Request[anubisv1.RevokeRequest]) (*connect.Response[anubisv1.RevokeResponse], error) {
	if _, err := h.eps.Revoke(ctx, authep.RevokeRequest{
		Token: req.Msg.Token, Hint: req.Msg.TokenTypeHint,
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RevokeResponse{}), nil
}
