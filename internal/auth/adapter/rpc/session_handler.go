package authrpc

import (
	"context"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	sessionapp "github.com/gsoultan/anubis/internal/auth/app/session"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authep "github.com/gsoultan/anubis/internal/auth/endpoint"
)

// SessionHandler implements anubisv1connect.SessionServiceHandler.
type SessionHandler struct {
	eps authep.SessionEndpoints
}

func NewSessionHandler(eps authep.SessionEndpoints) *SessionHandler {
	return &SessionHandler{eps: eps}
}

var _ anubisv1connect.SessionServiceHandler = (*SessionHandler)(nil)

func (h *SessionHandler) GetMe(ctx context.Context, _ *connect.Request[anubisv1.GetMeRequest]) (*connect.Response[anubisv1.GetMeResponse], error) {
	out, err := h.eps.GetMe(ctx, nil)
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	me := out.(*sessionapp.Me)
	return connect.NewResponse(&anubisv1.GetMeResponse{
		IdentityId: me.IdentityID, Tenant: me.Tenant, Realm: me.Realm,
		Username: me.Username, Email: me.Email, Roles: me.Roles,
		Permissions: me.Permissions, ActiveScopes: me.ActiveScopes,
		Amr: me.AMR, Ial: int32(me.IAL), SessionId: me.SessionID,
	}), nil
}

func (h *SessionHandler) ListSessions(ctx context.Context, _ *connect.Request[anubisv1.ListSessionsRequest]) (*connect.Response[anubisv1.ListSessionsResponse], error) {
	out, err := h.eps.ListSessions(ctx, nil)
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	sl := out.(authep.SessionList)
	list := sl.Sessions.([]authdomain.SessionInfo)
	resp := &anubisv1.ListSessionsResponse{}
	for _, s := range list {
		resp.Sessions = append(resp.Sessions, &anubisv1.Session{
			Id: s.ID, CreatedAt: s.CreatedAt.Unix(), LastSeenAt: s.LastSeenAt.Unix(),
			ExpiresAt: s.ExpiresAt.Unix(), Ip: s.IP, UserAgent: s.UserAgent,
			Amr: s.AMR, Current: s.ID == sl.CurrentID,
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *SessionHandler) RevokeSession(ctx context.Context, req *connect.Request[anubisv1.RevokeSessionRequest]) (*connect.Response[anubisv1.RevokeSessionResponse], error) {
	if _, err := h.eps.RevokeSession(ctx, req.Msg.SessionId); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RevokeSessionResponse{}), nil
}
