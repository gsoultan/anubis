package scopepg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/scope/adapter/postgres/rgen/scopenode"
	"github.com/gsoultan/anubis/internal/scope/adapter/postgres/rgen/scopenodetype"
	scopermquery "github.com/gsoultan/anubis/internal/scope/adapter/postgres/rquery"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

func (s *Repository) ListScopeNodeTypes(ctx context.Context, axis string) ([]scopedomain.ScopeNodeTypeRecord, error) {
	q := scopenodetype.New().Order(scopenodetype.AxisCode.Asc(), scopenodetype.Code.Asc())
	// An empty axis means every axis; the filter is omitted rather than
	// compared against '', which would match nothing.
	q = q.WhereIf(axis != "", scopenodetype.AxisCode.Eq(axis))
	rows, err := q.All(ctx, s.ex(ctx), nil)
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
	n := scopenodetype.Create()
	n.SetCode(t.Code)
	n.SetAxisCode(t.Axis)
	n.SetDisplayName(t.DisplayName)
	if len(t.ParentTypes) > 0 {
		// Left unset it takes the '{}' default. Assigning an empty slice
		// would write NULL over that, which the NOT NULL rejects.
		n.SetParentTypes(t.ParentTypes)
	}
	_, err := n.Insert(ctx, s.ex(ctx))
	return database.MapErr(err)
}

// ListScopeNodes is one keyset page of a tenant's tree for one axis.
func (s *Repository) ListScopeNodes(ctx context.Context, tenantID string, f scopedomain.ScopeNodeFilter) ([]scopedomain.ScopeNodeRecord, error) {
	f = f.Normalise()
	rows, err := scopermquery.ListScopeNodes.Query(ctx, s.ex(ctx),
		tenantID, f.Axis, optArg(f.ParentID), optArg(f.Query),
		optArg(f.AfterName), f.IncludeArchived, optArg(f.AfterID), f.Limit)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return nodeRecords(rows), nil
}

// optArg turns an absent filter into SQL NULL, which is what each of these
// predicates tests for. ” would be a value that matches nothing.
func optArg(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nodeRecords(rows []scopermquery.NodeRow) []scopedomain.ScopeNodeRecord {
	out := make([]scopedomain.ScopeNodeRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, nodeRecord(r))
	}
	return out
}

func nodeRecord(r scopermquery.NodeRow) scopedomain.ScopeNodeRecord {
	return scopeNodeFromRow(r.ID, r.AxisCode, r.NodeType,
		nstr(r.ParentID), nstr(r.ParentAxisCode),
		r.Slug, r.Name, nstr(r.ExternalRef), r.Status, r.IsAxisRoot, r.ChildCount)
}

func (s *Repository) ScopeNode(ctx context.Context, tenantID, id string) (*scopedomain.ScopeNodeRecord, error) {
	r, ok, err := scopermquery.GetScopeNode.One(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	rec := nodeRecord(r)
	return &rec, nil
}

// ScopeNodesByIDs resolves a HANDFUL of nodes — the names beside the grants on
// one screen. The console used to pull every node in every axis to render a
// dozen labels.
func (s *Repository) ScopeNodesByIDs(ctx context.Context, tenantID string, ids []string) ([]scopedomain.ScopeNodeRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := scopermquery.ScopeNodesByIDs.Query(ctx, s.ex(ctx), tenantID, ids)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return nodeRecords(rows), nil
}

func (s *Repository) ScopeNodeByRef(ctx context.Context, tenantID, axis, ref string) (*scopedomain.ScopeNodeRecord, error) {
	r, ok, err := scopermquery.GetScopeNodeByRef.One(ctx, s.ex(ctx), tenantID, axis, ref)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	rec := nodeRecord(r)
	return &rec, nil
}

// EnsureAxisRoot is idempotent by construction, which is why every path that
// needs a tree can call it without checking first.
func (s *Repository) EnsureAxisRoot(ctx context.Context, tenantID, axis string) (string, error) {
	row, _, err := scopermquery.EnsureAxisRoot.One(ctx, s.ex(ctx), tenantID, axis)
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.NodeID, nil
}

func (s *Repository) AddScopeNode(ctx context.Context, tenantID, axis, nodeType, parentID, slug, name, externalRef string) (string, error) {
	row, _, err := scopermquery.AddScopeNode.One(ctx, s.ex(ctx),
		tenantID, axis, nodeType, parentID, slug, name, externalRef)
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.NodeID, nil
}

func (s *Repository) MoveScopeNode(ctx context.Context, nodeID, newParentID string) error {
	_, _, err := scopermquery.MoveScopeNode.One(ctx, s.ex(ctx), nodeID, newParentID)
	return database.MapErr(err)
}

func (s *Repository) ArchiveScopeNode(ctx context.Context, tenantID, id string) error {
	n, err := scopermquery.ArchiveScopeNode.Exec(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		// Either it does not exist, or it is the axis root — which this
		// refuses, because archiving it would leave the axis with no tree.
		return database.NotFound()
	}
	return nil
}

func (s *Repository) RenameScopeNode(ctx context.Context, tenantID, id, name string) error {
	n, err := scopermquery.RenameScopeNode.Exec(ctx, s.ex(ctx), id, tenantID, name)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return database.NotFound()
	}
	return nil
}

// ScopeAncestors is the chain from the axis root down to a node.
func (s *Repository) ScopeAncestors(ctx context.Context, tenantID, nodeID string) ([]scopedomain.ScopeAncestor, error) {
	rows, err := scopermquery.ScopeAncestors.Query(ctx, s.ex(ctx), nodeID, tenantID)
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
				ParentID:   nstr(r.ParentID), ExternalRef: nstr(r.ExternalRef),
			},
		})
	}
	return out, nil
}

// CountActiveScopeNodes backs the console's overview: the structure's size.
func (s *Repository) CountActiveScopeNodes(ctx context.Context, tenantID string) (int64, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	n, err := scopenode.New().
		Where(scopenode.TenantID.Eq(tid), scopenode.Status.Eq("active")).
		Count(ctx, s.ex(ctx))
	if err != nil {
		return 0, database.MapErr(err)
	}
	return n, nil
}
