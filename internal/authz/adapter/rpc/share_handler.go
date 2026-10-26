package authzrpc

import (
	"context"
	"time"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	authzshare "github.com/gsoultan/anubis/internal/authz/app/share"
	"github.com/gsoultan/anubis/internal/authz/domain/share"
	authzep "github.com/gsoultan/anubis/internal/authz/endpoint"
)

type ShareHandler struct {
	eps authzep.ShareEndpoints
}

func NewShareHandler(eps authzep.ShareEndpoints) *ShareHandler { return &ShareHandler{eps: eps} }

var _ anubisv1connect.ShareServiceHandler = (*ShareHandler)(nil)

func (h *ShareHandler) Share(ctx context.Context, req *connect.Request[anubisv1.ShareRequest]) (*connect.Response[anubisv1.ShareResponse], error) {
	var until time.Time
	if req.Msg.ValidUntil != 0 {
		until = time.Unix(req.Msg.ValidUntil, 0)
	}
	out, err := h.eps.Share(ctx, authzshare.ShareInput{
		RecordKind: req.Msg.RecordKind, RecordRef: req.Msg.RecordRef, RecordScopes: req.Msg.RecordScopes,
		ToIdentityID: req.Msg.ToIdentityId, Permissions: req.Msg.Permissions,
		ValidUntil: until, Note: req.Msg.Note,
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.ShareResponse{Share: shareProto(out.(*share.Share))}), nil
}

func (h *ShareHandler) Unshare(ctx context.Context, req *connect.Request[anubisv1.UnshareRequest]) (*connect.Response[anubisv1.UnshareResponse], error) {
	if _, err := h.eps.Unshare(ctx, [2]string{req.Msg.ShareId, req.Msg.Reason}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.UnshareResponse{}), nil
}

func (h *ShareHandler) ListSharesOnRecord(ctx context.Context, req *connect.Request[anubisv1.ListSharesOnRecordRequest]) (*connect.Response[anubisv1.ListSharesOnRecordResponse], error) {
	out, err := h.eps.SharesOnRecord(ctx, authzep.WrapSharesOnRecord(req.Msg.RecordKind, req.Msg.RecordRef, req.Msg.RecordScopes))
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.ListSharesOnRecordResponse{Shares: sharesProto(out.([]share.Share))}), nil
}

func (h *ShareHandler) ListSharedWithMe(ctx context.Context, _ *connect.Request[anubisv1.ListSharedWithMeRequest]) (*connect.Response[anubisv1.ListSharedWithMeResponse], error) {
	out, err := h.eps.SharedWithMe(ctx, nil)
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.ListSharedWithMeResponse{Shares: sharesProto(out.([]share.Share))}), nil
}

func sharesProto(in []share.Share) []*anubisv1.RecordShare {
	out := make([]*anubisv1.RecordShare, 0, len(in))
	for i := range in {
		out = append(out, shareProto(&in[i]))
	}
	return out
}

func shareProto(s *share.Share) *anubisv1.RecordShare {
	kind, ref := share.SplitRecordKey(s.Record)
	return &anubisv1.RecordShare{
		Id: s.ID, RecordKind: kind, RecordRef: ref,
		FromIdentityId: s.FromIdentityID, ToIdentityId: s.ToIdentityID,
		Permissions: s.Permissions, Note: s.Note,
		CreatedAt: s.CreatedAt.Unix(), ValidUntil: s.ValidUntil.Unix(),
		RecordScopes: s.RecordScopes,
	}
}
