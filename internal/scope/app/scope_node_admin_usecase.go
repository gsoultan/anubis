package scopeapp

import (
	"context"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

type ScopeNodeAdminUsecase interface {
	ListScopeNodeTypes(ctx context.Context, axis string) ([]scopedomain.ScopeNodeTypeRecord, error)
	CreateScopeNodeType(ctx context.Context, t scopedomain.ScopeNodeTypeRecord) error
	// ListScopeNodes returns one keyset page plus the token to resume after
	// it, empty on the last page. See scopedomain.ScopeNodeFilter.
	ListScopeNodes(ctx context.Context, f scopedomain.ScopeNodeFilter) ([]scopedomain.ScopeNodeRecord, string, error)
	ScopeNode(ctx context.Context, id string) (*scopedomain.ScopeNodeRecord, error)
	// ScopeNodes resolves a bounded set of ids at once: the names beside one
	// screenful of grants, rather than every node in every axis.
	ScopeNodes(ctx context.Context, ids []string) ([]scopedomain.ScopeNodeRecord, error)
	ScopeAncestors(ctx context.Context, id string) ([]scopedomain.ScopeAncestor, error)
	CreateScopeNode(ctx context.Context, axis, nodeType, parentID, slug, name, externalRef string) (*scopedomain.ScopeNodeRecord, error)
	EnsureAxisRoot(ctx context.Context, axis string) (string, error)
	MoveScopeNode(ctx context.Context, nodeID, newParentID string) error
	ArchiveScopeNode(ctx context.Context, nodeID string) error
	// UpsertScopeNodes bulk-reconciles keyed on external_ref (parents first).
	UpsertScopeNodes(ctx context.Context, axis, defaultNodeType string, rows []SyncRowInput, dry bool) (reportJSON string, err error)
}
