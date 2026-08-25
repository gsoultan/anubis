package controlrpc

import (
	"context"
	"time"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	controlapp "github.com/gsoultan/anubis/internal/control/app"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	controlsvc "github.com/gsoultan/anubis/internal/control/service"
	"github.com/gsoultan/anubis/internal/platform/mw"
)

// PlatformAdminHandler implements anubisv1connect.PlatformAdminServiceHandler.
type PlatformAdminHandler struct {
	svc   controlsvc.ControlService
	f     mw.Factory
	clock func() time.Time
}

func NewPlatformAdminHandler(svc controlsvc.ControlService, f mw.Factory, now func() time.Time) *PlatformAdminHandler {
	return &PlatformAdminHandler{svc: svc, f: f, clock: now}
}

var _ anubisv1connect.PlatformAdminServiceHandler = (*PlatformAdminHandler)(nil)

func apiKeyProto(k controldomain.PlatformAPIKey) *anubisv1.PlatformApiKey {
	out := &anubisv1.PlatformApiKey{
		Id: k.ID, PlatformUserId: k.PlatformUserID, Username: k.Username,
		Label: k.Label, Lookup: k.Lookup,
		CreatedAt: k.CreatedAt.Unix(), ExpiresAt: k.ExpiresAt.Unix(),
	}
	if k.LastUsedAt != nil {
		out.LastUsedAt = k.LastUsedAt.Unix()
	}
	if k.RevokedAt != nil {
		out.RevokedAt = k.RevokedAt.Unix()
	}
	return out
}

func (h *PlatformAdminHandler) CreatePlatformApiKey(ctx context.Context,
	req *connect.Request[anubisv1.CreatePlatformApiKeyRequest],
) (*connect.Response[anubisv1.CreatePlatformApiKeyResponse], error) {
	type minted struct {
		full string
		rec  *controldomain.PlatformAPIKey
	}
	out, err := h.f.Do(ctx, "platform.apikey.create", func(ctx context.Context) (any, error) {
		full, rec, cerr := h.svc.CreateAPIKey(ctx, controlapp.CreateAPIKeyInput{
			OwnerID: req.Msg.OwnerId, Label: req.Msg.Label,
			ExpiresIn: time.Duration(req.Msg.ExpiresInDays) * 24 * time.Hour,
		})
		return minted{full: full, rec: rec}, cerr
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	m := out.(minted)
	return connect.NewResponse(&anubisv1.CreatePlatformApiKeyResponse{
		Key: apiKeyProto(*m.rec), ApiKey: m.full,
	}), nil
}

func (h *PlatformAdminHandler) ListPlatformApiKeys(ctx context.Context,
	_ *connect.Request[anubisv1.ListPlatformApiKeysRequest],
) (*connect.Response[anubisv1.ListPlatformApiKeysResponse], error) {
	out, err := h.f.Do(ctx, "platform.apikey.list", func(ctx context.Context) (any, error) {
		return h.svc.ListAPIKeys(ctx)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListPlatformApiKeysResponse{}
	for _, k := range out.([]controldomain.PlatformAPIKey) {
		resp.Keys = append(resp.Keys, apiKeyProto(k))
	}
	return connect.NewResponse(resp), nil
}

func (h *PlatformAdminHandler) RevokePlatformApiKey(ctx context.Context,
	req *connect.Request[anubisv1.RevokePlatformApiKeyRequest],
) (*connect.Response[anubisv1.RevokePlatformApiKeyResponse], error) {
	if _, err := h.f.Do(ctx, "platform.apikey.revoke", func(ctx context.Context) (any, error) {
		return nil, h.svc.RevokeAPIKey(ctx, req.Msg.Id)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RevokePlatformApiKeyResponse{}), nil
}

func (h *PlatformAdminHandler) ListOperators(ctx context.Context,
	req *connect.Request[anubisv1.ListOperatorsRequest],
) (*connect.Response[anubisv1.ListOperatorsResponse], error) {
	out, err := h.f.Do(ctx, "admin.platform.operators", func(ctx context.Context) (any, error) {
		return h.svc.ListOperators(ctx, controlapp.ListOperatorsInput{
			Query:    req.Msg.Query,
			Cursor:   req.Msg.PageToken,
			PageSize: int(req.Msg.PageSize),
		})
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	page := out.(*controldomain.Page)
	now := h.clock()
	resp := &anubisv1.ListOperatorsResponse{
		NextPageToken: page.NextCursor,
		Total:         int32(page.Total),
	}
	for _, o := range page.Users {
		op := &anubisv1.Operator{
			IdentityId: o.ID, Username: o.Username, Email: o.Email,
			Status: o.Status, CreatedAt: o.CreatedAt.Unix(), Owner: o.Owner(now),
			MfaEnrolled: o.MFAEnrolled(),
		}
		if o.LastLoginAt != nil {
			op.LastLoginAt = o.LastLoginAt.Unix()
		}
		for _, a := range o.Assignments {
			pa := &anubisv1.OperatorAssignment{
				Id: a.ID, TenantSlug: a.TenantSlug, Role: string(a.Role),
				Reason: a.Reason, CreatedAt: a.CreatedAt.Unix(),
			}
			if a.ValidUntil != nil {
				pa.ValidUntil = a.ValidUntil.Unix()
			}
			op.Assignments = append(op.Assignments, pa)
		}
		resp.Operators = append(resp.Operators, op)
	}
	return connect.NewResponse(resp), nil
}

func (h *PlatformAdminHandler) CreateOperator(ctx context.Context,
	req *connect.Request[anubisv1.CreateOperatorRequest],
) (*connect.Response[anubisv1.CreateOperatorResponse], error) {
	type created struct{ operator, assignment string }
	out, err := h.f.Do(ctx, "admin.platform.operator_create", func(ctx context.Context) (any, error) {
		id, aid, cerr := h.svc.CreateOperator(ctx, controlapp.CreateOperatorInput{
			Username:   req.Msg.Username,
			Email:      req.Msg.Email,
			Password:   req.Msg.Password,
			TenantSlug: req.Msg.TenantSlug,
			Role:       controldomain.OperatorRole(req.Msg.Role),
			Reason:     req.Msg.Reason,
		})
		return created{operator: id, assignment: aid}, cerr
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	c := out.(created)
	return connect.NewResponse(&anubisv1.CreateOperatorResponse{
		OperatorId: c.operator, AssignmentId: c.assignment,
	}), nil
}

func (h *PlatformAdminHandler) AssignOperator(ctx context.Context,
	req *connect.Request[anubisv1.AssignOperatorRequest],
) (*connect.Response[anubisv1.AssignOperatorResponse], error) {
	out, err := h.f.Do(ctx, "admin.platform.assign", func(ctx context.Context) (any, error) {
		in := controlapp.AssignOperatorInput{
			OperatorID:       req.Msg.OperatorId,
			OperatorUsername: req.Msg.OperatorUsername,
			TenantSlug:       req.Msg.TenantSlug,
			Role:             controldomain.OperatorRole(req.Msg.Role),
			Reason:           req.Msg.Reason,
		}
		if req.Msg.ValidUntil > 0 {
			t := time.Unix(req.Msg.ValidUntil, 0).UTC()
			in.ValidUntil = &t
		}
		return h.svc.AssignOperator(ctx, in)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.AssignOperatorResponse{AssignmentId: out.(string)}), nil
}

func (h *PlatformAdminHandler) RevokeAssignment(ctx context.Context,
	req *connect.Request[anubisv1.RevokeAssignmentRequest],
) (*connect.Response[anubisv1.RevokeAssignmentResponse], error) {
	if _, err := h.f.Do(ctx, "admin.platform.revoke", func(ctx context.Context) (any, error) {
		return nil, h.svc.RevokeAssignment(ctx, req.Msg.AssignmentId)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RevokeAssignmentResponse{}), nil
}

func (h *PlatformAdminHandler) SetOperatorStatus(ctx context.Context,
	req *connect.Request[anubisv1.SetOperatorStatusRequest],
) (*connect.Response[anubisv1.SetOperatorStatusResponse], error) {
	if _, err := h.f.Do(ctx, "admin.platform.operator_status", func(ctx context.Context) (any, error) {
		return nil, h.svc.SetOperatorStatus(ctx, req.Msg.OperatorId, req.Msg.Status)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.SetOperatorStatusResponse{}), nil
}
