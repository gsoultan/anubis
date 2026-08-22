package scopeapp

import (
	"context"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

type ScopeNodeAdminUsecase interface {
	ListScopeNodeTypes(ctx context.Context, axis string) ([]scopedomain.ScopeNodeTypeRecord, error)
	CreateScopeNodeType(ctx context.Context, t scopedomain.ScopeNodeTypeRecord) error
	ListScopeNodes(ctx context.Context, axis, parentID, query string, includeArchived bool) ([]scopedomain.ScopeNodeRecord, error)
	CreateScopeNode(ctx context.Context, axis, nodeType, parentID, slug, name, externalRef string) (*scopedomain.ScopeNodeRecord, error)
	EnsureAxisRoot(ctx context.Context, axis string) (string, error)
	MoveScopeNode(ctx context.Context, nodeID, newParentID string) error
	ArchiveScopeNode(ctx context.Context, nodeID string) error
	// UpsertScopeNodes bulk-reconciles keyed on external_ref (parents first).
	UpsertScopeNodes(ctx context.Context, axis, defaultNodeType string, rows []SyncRowInput, dry bool) (reportJSON string, err error)
}
