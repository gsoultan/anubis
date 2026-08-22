package connectrpc

import (
	"context"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	ep "github.com/gsoultan/anubis/internal/endpoint"
	"github.com/gsoultan/anubis/internal/usecase"
)

// AuthzHandler implements anubisv1connect.AuthzServiceHandler.
type AuthzHandler struct {
	eps ep.AuthzEndpoints
}

func NewAuthzHandler(eps ep.AuthzEndpoints) *AuthzHandler { return &AuthzHandler{eps: eps} }

var _ anubisv1connect.AuthzServiceHandler = (*AuthzHandler)(nil)

func (h *AuthzHandler) Authorize(ctx context.Context, req *connect.Request[anubisv1.AuthorizeRequest]) (*connect.Response[anubisv1.AuthorizeResponse], error) {
	out, err := h.eps.Authorize(ctx, usecase.AuthorizeInput{
		Subject: req.Msg.Subject, Permission: req.Msg.Permission,
		Scopes: req.Msg.Scopes, AMR: req.Msg.Amr, AuthTime: req.Msg.AuthTime,
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	d := out.(*usecase.Decision)
	return connect.NewResponse(&anubisv1.AuthorizeResponse{
		Allow: d.Allow, Reason: d.Reason, FailingAxis: d.FailingAxis,
		Message: d.Message, RequiredAmr: d.RequiredAMR, MaxAuthAge: d.MaxAuthAge,
		CurrentAmr: d.CurrentAMR, AuthAge: d.AuthAge,
	}), nil
}

func (h *AuthzHandler) Explain(ctx context.Context, req *connect.Request[anubisv1.ExplainRequest]) (*connect.Response[anubisv1.ExplainResponse], error) {
	out, err := h.eps.Explain(ctx, usecase.AuthorizeInput{
		Subject: req.Msg.Subject, Permission: req.Msg.Permission, Scopes: req.Msg.Scopes,
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	e := out.(*usecase.Explanation)
	return connect.NewResponse(&anubisv1.ExplainResponse{
		Allow: e.Allow, Reason: e.Reason, FailingAxis: e.FailingAxis,
		DetailJson: e.DetailJSON,
	}), nil
}

func (h *AuthzHandler) SwitchScope(ctx context.Context, req *connect.Request[anubisv1.SwitchScopeRequest]) (*connect.Response[anubisv1.SwitchScopeResponse], error) {
	out, err := h.eps.SwitchScope(ctx, req.Msg.Scopes)
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.SwitchScopeResponse{
		Tokens: pair(out.(*usecase.TokenPair)),
	}), nil
}
