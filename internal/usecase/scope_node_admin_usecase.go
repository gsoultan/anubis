package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/repository"
)

type ScopeNodeAdminUsecase interface {
	ListScopeNodeTypes(ctx context.Context, axis string) ([]repository.ScopeNodeTypeRecord, error)
	CreateScopeNodeType(ctx context.Context, t repository.ScopeNodeTypeRecord) error
	ListScopeNodes(ctx context.Context, axis, parentID, query string, includeArchived bool) ([]repository.ScopeNodeRecord, error)
	CreateScopeNode(ctx context.Context, axis, nodeType, parentID, slug, name, externalRef string) (*repository.ScopeNodeRecord, error)
	EnsureAxisRoot(ctx context.Context, axis string) (string, error)
	MoveScopeNode(ctx context.Context, nodeID, newParentID string) error
	ArchiveScopeNode(ctx context.Context, nodeID string) error
	// UpsertScopeNodes bulk-reconciles keyed on external_ref (parents first).
	UpsertScopeNodes(ctx context.Context, axis, defaultNodeType string, rows []SyncRowInput, dry bool) (reportJSON string, err error)
}
