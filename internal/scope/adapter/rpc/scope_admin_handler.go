package scoperpc

import (
	"context"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	"github.com/gsoultan/anubis/internal/platform/mw"
	scopeapp "github.com/gsoultan/anubis/internal/scope/app"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	scopesvc "github.com/gsoultan/anubis/internal/scope/service"
)

// ScopeAdminHandler implements anubisv1connect.ScopeAdminServiceHandler.
type ScopeAdminHandler struct {
	svc scopesvc.ScopeAdminService
	f   mw.Factory
}

func NewScopeAdminHandler(svc scopesvc.ScopeAdminService, f mw.Factory) *ScopeAdminHandler {
	return &ScopeAdminHandler{svc: svc, f: f}
}

var _ anubisv1connect.ScopeAdminServiceHandler = (*ScopeAdminHandler)(nil)

func axisProto(a scopedomain.ScopeAxisRecord) *anubisv1.ScopeAxis {
	return &anubisv1.ScopeAxis{
		Code: a.Code, DisplayName: a.DisplayName, DefaultEffect: a.DefaultEffect,
		Status: a.Status, SortOrder: int32(a.SortOrder),
		ResolutionJson: string(a.Resolution), UiSchemaJson: string(a.UISchema),
	}
}

func axisRecord(a *anubisv1.ScopeAxis) scopedomain.ScopeAxisRecord {
	return scopedomain.ScopeAxisRecord{
		Code: a.Code, DisplayName: a.DisplayName, DefaultEffect: a.DefaultEffect,
		Status: a.Status, SortOrder: int(a.SortOrder),
		Resolution: []byte(a.ResolutionJson), UISchema: []byte(a.UiSchemaJson),
	}
}

func nodeProto(n scopedomain.ScopeNodeRecord) *anubisv1.ScopeNode {
	return &anubisv1.ScopeNode{
		Id: n.ID, Axis: n.Axis, NodeType: n.NodeType, ParentId: n.ParentID,
		Slug: n.Slug, Name: n.Name, ExternalRef: n.ExternalRef,
		Status: n.Status, IsAxisRoot: n.IsAxisRoot,
	}
}

func syncRows(rows []*anubisv1.SyncRow) []scopeapp.SyncRowInput {
	out := make([]scopeapp.SyncRowInput, 0, len(rows))
	for _, r := range rows {
		out = append(out, scopeapp.SyncRowInput{
			Ref: r.Ref, ParentRef: r.ParentRef, Name: r.Name, NodeType: r.NodeType,
		})
	}
	return out
}

func (h *ScopeAdminHandler) ListScopeAxes(ctx context.Context, _ *connect.Request[anubisv1.ListScopeAxesRequest]) (*connect.Response[anubisv1.ListScopeAxesResponse], error) {
	out, err := h.f.Do(ctx, "admin.scope.axes", func(ctx context.Context) (any, error) {
		return h.svc.ListScopeAxes(ctx)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListScopeAxesResponse{}
	for _, a := range out.([]scopedomain.ScopeAxisRecord) {
		resp.Axes = append(resp.Axes, axisProto(a))
	}
	return connect.NewResponse(resp), nil
}

func (h *ScopeAdminHandler) CreateScopeAxis(ctx context.Context, req *connect.Request[anubisv1.CreateScopeAxisRequest]) (*connect.Response[anubisv1.CreateScopeAxisResponse], error) {
	out, err := h.f.Do(ctx, "admin.scope.axis_create", func(ctx context.Context) (any, error) {
		return h.svc.CreateScopeAxis(ctx, axisRecord(req.Msg.Axis))
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.CreateScopeAxisResponse{
		Axis: axisProto(*out.(*scopedomain.ScopeAxisRecord)),
	}), nil
}

func (h *ScopeAdminHandler) UpdateScopeAxis(ctx context.Context, req *connect.Request[anubisv1.UpdateScopeAxisRequest]) (*connect.Response[anubisv1.UpdateScopeAxisResponse], error) {
	out, err := h.f.Do(ctx, "admin.scope.axis_update", func(ctx context.Context) (any, error) {
		return h.svc.UpdateScopeAxis(ctx, axisRecord(req.Msg.Axis))
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.UpdateScopeAxisResponse{
		Axis: axisProto(*out.(*scopedomain.ScopeAxisRecord)),
	}), nil
}

func (h *ScopeAdminHandler) StrictDryRun(ctx context.Context, req *connect.Request[anubisv1.StrictDryRunRequest]) (*connect.Response[anubisv1.StrictDryRunResponse], error) {
	out, err := h.f.Do(ctx, "admin.scope.strict_dry_run", func(ctx context.Context) (any, error) {
		sampled, wouldDeny, examples, err := h.svc.StrictDryRun(ctx, req.Msg.Axis, int(req.Msg.SampleSize))
		if err != nil {
			return nil, err
		}
		return &anubisv1.StrictDryRunResponse{
			Sampled: int32(sampled), WouldDeny: int32(wouldDeny), ExamplesJson: examples,
		}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.StrictDryRunResponse)), nil
}

func (h *ScopeAdminHandler) ListScopeNodeTypes(ctx context.Context, req *connect.Request[anubisv1.ListScopeNodeTypesRequest]) (*connect.Response[anubisv1.ListScopeNodeTypesResponse], error) {
	out, err := h.f.Do(ctx, "admin.scope.node_types", func(ctx context.Context) (any, error) {
		return h.svc.ListScopeNodeTypes(ctx, req.Msg.Axis)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListScopeNodeTypesResponse{}
	for _, t := range out.([]scopedomain.ScopeNodeTypeRecord) {
		resp.Types = append(resp.Types, &anubisv1.ScopeNodeType{
			Code: t.Code, Axis: t.Axis, DisplayName: t.DisplayName, ParentTypes: t.ParentTypes,
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *ScopeAdminHandler) CreateScopeNodeType(ctx context.Context, req *connect.Request[anubisv1.CreateScopeNodeTypeRequest]) (*connect.Response[anubisv1.CreateScopeNodeTypeResponse], error) {
	if _, err := h.f.Do(ctx, "admin.scope.node_type_create", func(ctx context.Context) (any, error) {
		t := req.Msg.Type
		return nil, h.svc.CreateScopeNodeType(ctx, scopedomain.ScopeNodeTypeRecord{
			Code: t.Code, Axis: t.Axis, DisplayName: t.DisplayName, ParentTypes: t.ParentTypes,
		})
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.CreateScopeNodeTypeResponse{Type: req.Msg.Type}), nil
}

func (h *ScopeAdminHandler) ListScopeNodes(ctx context.Context, req *connect.Request[anubisv1.ListScopeNodesRequest]) (*connect.Response[anubisv1.ListScopeNodesResponse], error) {
	out, err := h.f.Do(ctx, "admin.scope.nodes", func(ctx context.Context) (any, error) {
		return h.svc.ListScopeNodes(ctx, req.Msg.Axis, req.Msg.ParentId, req.Msg.Query, req.Msg.IncludeArchived)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListScopeNodesResponse{}
	for _, n := range out.([]scopedomain.ScopeNodeRecord) {
		resp.Nodes = append(resp.Nodes, nodeProto(n))
	}
	return connect.NewResponse(resp), nil
}

func (h *ScopeAdminHandler) CreateScopeNode(ctx context.Context, req *connect.Request[anubisv1.CreateScopeNodeRequest]) (*connect.Response[anubisv1.CreateScopeNodeResponse], error) {
	out, err := h.f.Do(ctx, "admin.scope.node_create", func(ctx context.Context) (any, error) {
		return h.svc.CreateScopeNode(ctx, req.Msg.Axis, req.Msg.NodeType,
			req.Msg.ParentId, req.Msg.Slug, req.Msg.Name, req.Msg.ExternalRef)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.CreateScopeNodeResponse{
		Node: nodeProto(*out.(*scopedomain.ScopeNodeRecord)),
	}), nil
}

func (h *ScopeAdminHandler) EnsureAxisRoot(ctx context.Context, req *connect.Request[anubisv1.EnsureAxisRootRequest]) (*connect.Response[anubisv1.EnsureAxisRootResponse], error) {
	out, err := h.f.Do(ctx, "admin.scope.ensure_root", func(ctx context.Context) (any, error) {
		return h.svc.EnsureAxisRoot(ctx, req.Msg.Axis)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.EnsureAxisRootResponse{NodeId: out.(string)}), nil
}

func (h *ScopeAdminHandler) MoveScopeNode(ctx context.Context, req *connect.Request[anubisv1.MoveScopeNodeRequest]) (*connect.Response[anubisv1.MoveScopeNodeResponse], error) {
	if _, err := h.f.Do(ctx, "admin.scope.node_move", func(ctx context.Context) (any, error) {
		return nil, h.svc.MoveScopeNode(ctx, req.Msg.NodeId, req.Msg.NewParentId)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.MoveScopeNodeResponse{}), nil
}

func (h *ScopeAdminHandler) ArchiveScopeNode(ctx context.Context, req *connect.Request[anubisv1.ArchiveScopeNodeRequest]) (*connect.Response[anubisv1.ArchiveScopeNodeResponse], error) {
	if _, err := h.f.Do(ctx, "admin.scope.node_archive", func(ctx context.Context) (any, error) {
		return nil, h.svc.ArchiveScopeNode(ctx, req.Msg.NodeId)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.ArchiveScopeNodeResponse{}), nil
}

func (h *ScopeAdminHandler) UpsertScopeNodes(ctx context.Context, req *connect.Request[anubisv1.UpsertScopeNodesRequest]) (*connect.Response[anubisv1.UpsertScopeNodesResponse], error) {
	out, err := h.f.Do(ctx, "admin.scope.bulk_upsert", func(ctx context.Context) (any, error) {
		return h.svc.UpsertScopeNodes(ctx, req.Msg.Axis, req.Msg.DefaultNodeType,
			syncRows(req.Msg.Rows), req.Msg.Dry)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.UpsertScopeNodesResponse{ReportJson: out.(string)}), nil
}

func (h *ScopeAdminHandler) ListSyncSources(ctx context.Context, _ *connect.Request[anubisv1.ListSyncSourcesRequest]) (*connect.Response[anubisv1.ListSyncSourcesResponse], error) {
	out, err := h.f.Do(ctx, "admin.sync.sources", func(ctx context.Context) (any, error) {
		return h.svc.ListSyncSources(ctx)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListSyncSourcesResponse{}
	for _, s := range out.([]scopedomain.SyncSourceRecord) {
		src := &anubisv1.SyncSource{
			Id: s.ID, Axis: s.Axis, Kind: s.Kind, Status: s.Status,
			ConfigJson: string(s.Config),
		}
		if s.LastRunAt != nil {
			src.LastRunAt = s.LastRunAt.Unix()
		}
		resp.Sources = append(resp.Sources, src)
	}
	return connect.NewResponse(resp), nil
}

func (h *ScopeAdminHandler) CreateSyncSource(ctx context.Context, req *connect.Request[anubisv1.CreateSyncSourceRequest]) (*connect.Response[anubisv1.CreateSyncSourceResponse], error) {
	out, err := h.f.Do(ctx, "admin.sync.source_create", func(ctx context.Context) (any, error) {
		s := req.Msg.Source
		return h.svc.CreateSyncSource(ctx, scopedomain.SyncSourceRecord{
			Axis: s.Axis, Kind: s.Kind, Config: []byte(s.ConfigJson),
		})
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	s := out.(*scopedomain.SyncSourceRecord)
	return connect.NewResponse(&anubisv1.CreateSyncSourceResponse{
		Source: &anubisv1.SyncSource{
			Id: s.ID, Axis: s.Axis, Kind: s.Kind, Status: s.Status, ConfigJson: string(s.Config),
		},
	}), nil
}

func (h *ScopeAdminHandler) UpdateSyncSource(ctx context.Context, req *connect.Request[anubisv1.UpdateSyncSourceRequest]) (*connect.Response[anubisv1.UpdateSyncSourceResponse], error) {
	out, err := h.f.Do(ctx, "admin.sync.source_update", func(ctx context.Context) (any, error) {
		s := req.Msg.Source
		return h.svc.UpdateSyncSource(ctx, scopedomain.SyncSourceRecord{
			ID: s.Id, Status: s.Status, Config: []byte(s.ConfigJson),
		})
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	s := out.(*scopedomain.SyncSourceRecord)
	return connect.NewResponse(&anubisv1.UpdateSyncSourceResponse{
		Source: &anubisv1.SyncSource{
			Id: s.ID, Axis: s.Axis, Kind: s.Kind, Status: s.Status, ConfigJson: string(s.Config),
		},
	}), nil
}

func (h *ScopeAdminHandler) RunSync(ctx context.Context, req *connect.Request[anubisv1.RunSyncRequest]) (*connect.Response[anubisv1.RunSyncResponse], error) {
	out, err := h.f.Do(ctx, "admin.sync.run", func(ctx context.Context) (any, error) {
		return h.svc.RunSync(ctx, req.Msg.SourceId, syncRows(req.Msg.Rows), req.Msg.Dry)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RunSyncResponse{ReportJson: out.(string)}), nil
}

func (h *ScopeAdminHandler) GetScopeNode(ctx context.Context, req *connect.Request[anubisv1.GetScopeNodeRequest]) (*connect.Response[anubisv1.GetScopeNodeResponse], error) {
	out, err := h.f.Do(ctx, "admin.scope.node", func(ctx context.Context) (any, error) {
		return h.svc.ScopeNode(ctx, req.Msg.Id)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	n, _ := out.(*scopedomain.ScopeNodeRecord)
	if n == nil {
		return connect.NewResponse(&anubisv1.GetScopeNodeResponse{}), nil
	}
	return connect.NewResponse(&anubisv1.GetScopeNodeResponse{Node: nodeProto(*n)}), nil
}

func (h *ScopeAdminHandler) ScopeAncestors(ctx context.Context, req *connect.Request[anubisv1.ScopeAncestorsRequest]) (*connect.Response[anubisv1.ScopeAncestorsResponse], error) {
	out, err := h.f.Do(ctx, "admin.scope.ancestors", func(ctx context.Context) (any, error) {
		return h.svc.ScopeAncestors(ctx, req.Msg.Id)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ScopeAncestorsResponse{}
	for _, a := range out.([]scopedomain.ScopeAncestor) {
		resp.Ancestors = append(resp.Ancestors, &anubisv1.ScopeAncestor{
			Node: nodeProto(a.Node), Depth: int32(a.Depth),
		})
	}
	return connect.NewResponse(resp), nil
}
