package authzrpc

import (
	"context"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	authrpc "github.com/gsoultan/anubis/internal/auth/adapter/rpc"
	authapp "github.com/gsoultan/anubis/internal/auth/app"
	authzapp "github.com/gsoultan/anubis/internal/authz/app"
	authzreach "github.com/gsoultan/anubis/internal/authz/app/reach"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/authz/domain/granted"
	authzep "github.com/gsoultan/anubis/internal/authz/endpoint"
)

// AuthzHandler implements anubisv1connect.AuthzServiceHandler.
type AuthzHandler struct {
	eps authzep.AuthzEndpoints
}

func NewAuthzHandler(eps authzep.AuthzEndpoints) *AuthzHandler { return &AuthzHandler{eps: eps} }

var _ anubisv1connect.AuthzServiceHandler = (*AuthzHandler)(nil)

func (h *AuthzHandler) Authorize(ctx context.Context, req *connect.Request[anubisv1.AuthorizeRequest]) (*connect.Response[anubisv1.AuthorizeResponse], error) {
	out, err := h.eps.Authorize(ctx, authzapp.AuthorizeInput{
		Subject: req.Msg.Subject, Permission: req.Msg.Permission,
		Scopes: req.Msg.Scopes, AMR: req.Msg.Amr, AuthTime: req.Msg.AuthTime,
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	d := out.(*authzdomain.Decision)
	return connect.NewResponse(&anubisv1.AuthorizeResponse{
		Allow: d.Allow, Reason: d.Reason, FailingAxis: d.FailingAxis,
		Message: d.Message, RequiredAmr: d.RequiredAMR, MaxAuthAge: d.MaxAuthAge,
		CurrentAmr: d.CurrentAMR, AuthAge: d.AuthAge,
	}), nil
}

func (h *AuthzHandler) Explain(ctx context.Context, req *connect.Request[anubisv1.ExplainRequest]) (*connect.Response[anubisv1.ExplainResponse], error) {
	out, err := h.eps.Explain(ctx, authzapp.AuthorizeInput{
		Subject: req.Msg.Subject, Permission: req.Msg.Permission, Scopes: req.Msg.Scopes,
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	e := out.(*authzdomain.Explanation)
	return connect.NewResponse(&anubisv1.ExplainResponse{
		Allow: e.Allow, Reason: e.Reason, FailingAxis: e.FailingAxis,
		DetailJson: e.DetailJSON,
	}), nil
}

func (h *AuthzHandler) SwitchScope(ctx context.Context, req *connect.Request[anubisv1.SwitchScopeRequest]) (*connect.Response[anubisv1.SwitchScopeResponse], error) {
	out, err := h.eps.SwitchScope(ctx, req.Msg.Scopes)
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.SwitchScopeResponse{
		Tokens: authrpc.TokenPairProto(out.(*authapp.TokenPair)),
	}), nil
}

func (h *AuthzHandler) ListGrantedNodes(ctx context.Context, req *connect.Request[anubisv1.ListGrantedNodesRequest]) (*connect.Response[anubisv1.ListGrantedNodesResponse], error) {
	out, err := h.eps.GrantedNodes(ctx, authzreach.NodesInput{
		Subject: req.Msg.Subject, Permission: req.Msg.Permission,
		Axis: req.Msg.Axis, Scopes: req.Msg.Scopes,
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	g := out.(*granted.Nodes)
	return connect.NewResponse(&anubisv1.ListGrantedNodesResponse{
		Everywhere: g.Everywhere, OwnRecords: g.OwnRecords,
		Nodes: grantedNodesProto(g.Nodes), AsOf: g.AsOf.Unix(),
	}), nil
}

func grantedNodesProto(nodes []granted.Node) []*anubisv1.GrantedNode {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]*anubisv1.GrantedNode, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, &anubisv1.GrantedNode{
			NodeId: n.NodeID, Inherit: n.Inherit, Except: grantedNodesProto(n.Except),
		})
	}
	return out
}
