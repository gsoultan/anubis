package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) ListScopeNodeTypes(ctx context.Context, axis string) ([]repository.ScopeNodeTypeRecord, error) {
	rows, err := s.q(ctx).ListScopeNodeTypes(ctx, optStr(axis))
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.ScopeNodeTypeRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.ScopeNodeTypeRecord{
			Code: r.Code, Axis: r.AxisCode, DisplayName: r.DisplayName,
			ParentTypes: r.ParentTypes,
		})
	}
	return out, nil
}

func (s *Store) CreateScopeNodeType(ctx context.Context, t repository.ScopeNodeTypeRecord) error {
	_, err := s.q(ctx).CreateScopeNodeType(ctx, gen.CreateScopeNodeTypeParams{
		Code: t.Code, AxisCode: t.Axis, DisplayName: t.DisplayName,
		ParentTypes: emptyIfNil(t.ParentTypes),
	})
	return mapErr(err)
}

func (s *Store) ListScopeNodes(ctx context.Context, tenantID, axis, parentID, query string, includeArchived bool) ([]repository.ScopeNodeRecord, error) {
	rows, err := s.q(ctx).ListScopeNodes(ctx, gen.ListScopeNodesParams{
		TenantID: tenantID, AxisCode: axis,
		ParentID: optStr(parentID), Query: optStr(query),
		IncludeArchived: includeArchived,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.ScopeNodeRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, scopeNodeFromRow(r.ID, r.AxisCode, r.NodeType,
			deref(r.ParentID), r.Slug, r.Name, deref(r.ExternalRef), r.Status, r.IsAxisRoot))
	}
	return out, nil
}

func (s *Store) ScopeNode(ctx context.Context, tenantID, id string) (*repository.ScopeNodeRecord, error) {
	r, err := s.q(ctx).GetScopeNode(ctx, gen.GetScopeNodeParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, mapErr(err)
	}
	rec := scopeNodeFromRow(r.ID, r.AxisCode, r.NodeType, deref(r.ParentID),
		r.Slug, r.Name, deref(r.ExternalRef), r.Status, r.IsAxisRoot)
	return &rec, nil
}

func (s *Store) ScopeNodeByRef(ctx context.Context, tenantID, axis, ref string) (*repository.ScopeNodeRecord, error) {
	r, err := s.q(ctx).GetScopeNodeByRef(ctx, gen.GetScopeNodeByRefParams{
		TenantID: tenantID, AxisCode: axis, ExternalRef: optStr(ref),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	rec := scopeNodeFromRow(r.ID, r.AxisCode, r.NodeType, deref(r.ParentID),
		r.Slug, r.Name, deref(r.ExternalRef), r.Status, r.IsAxisRoot)
	return &rec, nil
}

func (s *Store) EnsureAxisRoot(ctx context.Context, tenantID, axis string) (string, error) {
	id, err := s.q(ctx).EnsureAxisRoot(ctx, gen.EnsureAxisRootParams{
		TenantID: tenantID, AxisCode: axis,
	})
	return id, mapErr(err)
}

func (s *Store) AddScopeNode(ctx context.Context, tenantID, axis, nodeType, parentID, slug, name, externalRef string) (string, error) {
	id, err := s.q(ctx).AddScopeNode(ctx, gen.AddScopeNodeParams{
		TenantID: tenantID, AxisCode: axis, NodeType: nodeType,
		ParentID: parentID, Slug: slug, Name: name, ExternalRef: externalRef,
	})
	return id, mapErr(err)
}

func (s *Store) MoveScopeNode(ctx context.Context, nodeID, newParentID string) error {
	return mapErr(s.q(ctx).MoveScopeNode(ctx, gen.MoveScopeNodeParams{
		NodeID: nodeID, NewParentID: newParentID,
	}))
}

func (s *Store) ArchiveScopeNode(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).ArchiveScopeNode(ctx, gen.ArchiveScopeNodeParams{ID: id, TenantID: tenantID})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return notFoundErr()
	}
	return nil
}

func (s *Store) RenameScopeNode(ctx context.Context, tenantID, id, name string) error {
	n, err := s.q(ctx).RenameScopeNode(ctx, gen.RenameScopeNodeParams{ID: id, TenantID: tenantID, Name: name})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return notFoundErr()
	}
	return nil
}
