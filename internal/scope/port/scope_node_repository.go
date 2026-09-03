package scopeport

import (
	"context"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

type ScopeNodeRepository interface {
	ListScopeNodeTypes(ctx context.Context, axis string) ([]scopedomain.ScopeNodeTypeRecord, error)
	CreateScopeNodeType(ctx context.Context, t scopedomain.ScopeNodeTypeRecord) error
	// ListScopeNodes returns ONE KEYSET PAGE, at most f.Limit rows. Callers
	// that need the whole axis must page with ScopeNodeFilter.After until a
	// short page comes back — see EachScopeNode.
	ListScopeNodes(ctx context.Context, tenantID string, f scopedomain.ScopeNodeFilter) ([]scopedomain.ScopeNodeRecord, error)
	ScopeNode(ctx context.Context, tenantID, id string) (*scopedomain.ScopeNodeRecord, error)
	// ScopeNodesByIDs resolves a bounded set in one round trip — the names
	// beside one screenful of grants, not the whole axis.
	ScopeNodesByIDs(ctx context.Context, tenantID string, ids []string) ([]scopedomain.ScopeNodeRecord, error)
	ScopeNodeByRef(ctx context.Context, tenantID, axis, ref string) (*scopedomain.ScopeNodeRecord, error)
	ScopeAncestors(ctx context.Context, nodeID string) ([]scopedomain.ScopeAncestor, error)
	EnsureAxisRoot(ctx context.Context, tenantID, axis string) (string, error)
	AddScopeNode(ctx context.Context, tenantID, axis, nodeType, parentID, slug, name, externalRef string) (string, error)
	MoveScopeNode(ctx context.Context, nodeID, newParentID string) error
	ArchiveScopeNode(ctx context.Context, tenantID, id string) error
	RenameScopeNode(ctx context.Context, tenantID, id, name string) error
}
