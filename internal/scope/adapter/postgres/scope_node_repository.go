package scopepg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
	gen "github.com/gsoultan/anubis/internal/scope/adapter/postgres/gen"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

func (s *Repository) ListScopeNodeTypes(ctx context.Context, axis string) ([]scopedomain.ScopeNodeTypeRecord, error) {
	rows, err := s.q(ctx).ListScopeNodeTypes(ctx, database.OptStr(axis))
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]scopedomain.ScopeNodeTypeRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, scopedomain.ScopeNodeTypeRecord{
			Code: r.Code, Axis: r.AxisCode, DisplayName: r.DisplayName,
			ParentTypes: r.ParentTypes,
		})
	}
	return out, nil
}

func (s *Repository) CreateScopeNodeType(ctx context.Context, t scopedomain.ScopeNodeTypeRecord) error {
	_, err := s.q(ctx).CreateScopeNodeType(ctx, gen.CreateScopeNodeTypeParams{
		Code: t.Code, AxisCode: t.Axis, DisplayName: t.DisplayName,
		ParentTypes: database.EmptyIfNil(t.ParentTypes),
	})
	return database.MapErr(err)
}

func (s *Repository) ListScopeNodes(ctx context.Context, tenantID, axis, parentID, query string, includeArchived bool) ([]scopedomain.ScopeNodeRecord, error) {
	rows, err := s.q(ctx).ListScopeNodes(ctx, gen.ListScopeNodesParams{
		TenantID: tenantID, AxisCode: axis,
		ParentID: database.OptStr(parentID), Query: database.OptStr(query),
		IncludeArchived: includeArchived,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]scopedomain.ScopeNodeRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, scopeNodeFromRow(r.ID, r.AxisCode, r.NodeType,
			database.Deref(r.ParentID), r.Slug, r.Name, database.Deref(r.ExternalRef), r.Status, r.IsAxisRoot))
	}
	return out, nil
}

func (s *Repository) ScopeNode(ctx context.Context, tenantID, id string) (*scopedomain.ScopeNodeRecord, error) {
	r, err := s.q(ctx).GetScopeNode(ctx, gen.GetScopeNodeParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, database.MapErr(err)
	}
	rec := scopeNodeFromRow(r.ID, r.AxisCode, r.NodeType, database.Deref(r.ParentID),
		r.Slug, r.Name, database.Deref(r.ExternalRef), r.Status, r.IsAxisRoot)
	return &rec, nil
}

func (s *Repository) ScopeNodeByRef(ctx context.Context, tenantID, axis, ref string) (*scopedomain.ScopeNodeRecord, error) {
	r, err := s.q(ctx).GetScopeNodeByRef(ctx, gen.GetScopeNodeByRefParams{
		TenantID: tenantID, AxisCode: axis, ExternalRef: database.OptStr(ref),
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	rec := scopeNodeFromRow(r.ID, r.AxisCode, r.NodeType, database.Deref(r.ParentID),
		r.Slug, r.Name, database.Deref(r.ExternalRef), r.Status, r.IsAxisRoot)
	return &rec, nil
}

func (s *Repository) EnsureAxisRoot(ctx context.Context, tenantID, axis string) (string, error) {
	id, err := s.q(ctx).EnsureAxisRoot(ctx, gen.EnsureAxisRootParams{
		TenantID: tenantID, AxisCode: axis,
	})
	return id, database.MapErr(err)
}

func (s *Repository) AddScopeNode(ctx context.Context, tenantID, axis, nodeType, parentID, slug, name, externalRef string) (string, error) {
	id, err := s.q(ctx).AddScopeNode(ctx, gen.AddScopeNodeParams{
		TenantID: tenantID, AxisCode: axis, NodeType: nodeType,
		ParentID: parentID, Slug: slug, Name: name, ExternalRef: externalRef,
	})
	return id, database.MapErr(err)
}

func (s *Repository) MoveScopeNode(ctx context.Context, nodeID, newParentID string) error {
	return database.MapErr(s.q(ctx).MoveScopeNode(ctx, gen.MoveScopeNodeParams{
		NodeID: nodeID, NewParentID: newParentID,
	}))
}

func (s *Repository) ArchiveScopeNode(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).ArchiveScopeNode(ctx, gen.ArchiveScopeNodeParams{ID: id, TenantID: tenantID})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return database.NotFound()
	}
	return nil
}

func (s *Repository) RenameScopeNode(ctx context.Context, tenantID, id, name string) error {
	n, err := s.q(ctx).RenameScopeNode(ctx, gen.RenameScopeNodeParams{ID: id, TenantID: tenantID, Name: name})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return database.NotFound()
	}
	return nil
}

// ScopeAncestors is the chain from the axis root down to a node.
func (s *Repository) ScopeAncestors(ctx context.Context, nodeID string) ([]scopedomain.ScopeAncestor, error) {
	rows, err := s.q(ctx).ScopeAncestors(ctx, nodeID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]scopedomain.ScopeAncestor, 0, len(rows))
	for _, r := range rows {
		out = append(out, scopedomain.ScopeAncestor{
			Depth: int(r.Depth),
			Node: scopedomain.ScopeNodeRecord{
				ID: r.ID, Axis: r.AxisCode, NodeType: r.NodeType,
				Slug: r.Slug, Name: r.Name, Status: r.Status,
				IsAxisRoot: r.IsAxisRoot,
				ParentID:   derefStr(r.ParentID), ExternalRef: derefStr(r.ExternalRef),
			},
		})
	}
	return out, nil
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// CountActiveScopeNodes backs the console's overview: the structure's size.
func (s *Repository) CountActiveScopeNodes(ctx context.Context, tenantID string) (int64, error) {
	n, err := s.q(ctx).CountActiveScopeNodes(ctx, tenantID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	return n, nil
}
