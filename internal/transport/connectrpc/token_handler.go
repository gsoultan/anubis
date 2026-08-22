package connectrpc

import (
	"context"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/domain"
	ep "github.com/gsoultan/anubis/internal/endpoint"
	"github.com/gsoultan/anubis/internal/usecase"
)

// TokenHandler implements anubisv1connect.TokenServiceHandler.
type TokenHandler struct {
	eps ep.TokenEndpoints
}

func NewTokenHandler(eps ep.TokenEndpoints) *TokenHandler { return &TokenHandler{eps: eps} }

var _ anubisv1connect.TokenServiceHandler = (*TokenHandler)(nil)

// Introspect is service-auth only (api.md): most applications should verify
// offline; whoever calls this is trusted with session-state answers.
func (h *TokenHandler) Introspect(ctx context.Context, req *connect.Request[anubisv1.IntrospectRequest]) (*connect.Response[anubisv1.IntrospectResponse], error) {
	if p, ok := authctx.From(ctx); !ok || !p.Service {
		return nil, toConnectErr(ctx, domain.ErrUnauthenticated)
	}
	out, err := h.eps.Introspect(ctx, req.Msg.Token)
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	r := out.(*usecase.IntrospectResult)
	return connect.NewResponse(&anubisv1.IntrospectResponse{
		Active: r.Active, Sub: r.Subject, Sid: r.Session, Tid: r.Tenant,
		Realm: r.Realm, Roles: r.Roles, Scopes: r.Scopes, Amr: r.AMR,
		Aud: r.Audience, Exp: r.Expires, AuthTime: r.AuthTime,
		Ial: int32(r.IAL), Epoch: int32(r.Epoch),
	}), nil
}

func (h *TokenHandler) Revoke(ctx context.Context, req *connect.Request[anubisv1.RevokeRequest]) (*connect.Response[anubisv1.RevokeResponse], error) {
	if _, err := h.eps.Revoke(ctx, ep.RevokeRequest{
		Token: req.Msg.Token, Hint: req.Msg.TokenTypeHint,
	}); err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RevokeResponse{}), nil
}
