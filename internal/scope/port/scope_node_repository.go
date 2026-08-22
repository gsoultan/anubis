package scopeport

import (
	"context"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

type ScopeNodeRepository interface {
	ListScopeNodeTypes(ctx context.Context, axis string) ([]scopedomain.ScopeNodeTypeRecord, error)
	CreateScopeNodeType(ctx context.Context, t scopedomain.ScopeNodeTypeRecord) error
	ListScopeNodes(ctx context.Context, tenantID, axis, parentID, query string, includeArchived bool) ([]scopedomain.ScopeNodeRecord, error)
	ScopeNode(ctx context.Context, tenantID, id string) (*scopedomain.ScopeNodeRecord, error)
	ScopeNodeByRef(ctx context.Context, tenantID, axis, ref string) (*scopedomain.ScopeNodeRecord, error)
	EnsureAxisRoot(ctx context.Context, tenantID, axis string) (string, error)
	AddScopeNode(ctx context.Context, tenantID, axis, nodeType, parentID, slug, name, externalRef string) (string, error)
	MoveScopeNode(ctx context.Context, nodeID, newParentID string) error
	ArchiveScopeNode(ctx context.Context, tenantID, id string) error
	RenameScopeNode(ctx context.Context, tenantID, id, name string) error
}
