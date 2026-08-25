package scopeport

import (
	"context"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

type ScopeSyncRepository interface {
	ListSyncSources(ctx context.Context, tenantID string) ([]scopedomain.SyncSourceRecord, error)
	SyncSource(ctx context.Context, tenantID, id string) (*scopedomain.SyncSourceRecord, error)
	CreateSyncSource(ctx context.Context, tenantID string, s scopedomain.SyncSourceRecord) (string, error)
	UpdateSyncSource(ctx context.Context, tenantID string, s scopedomain.SyncSourceRecord) error
	ScopeSyncApply(ctx context.Context, sourceID string, rows []byte, dry bool) (string, error)
	// ListSyncRuns is the recorded history of one feed, newest first.
	ListSyncRuns(ctx context.Context, tenantID, sourceID string, limit int32) ([]scopedomain.SyncRun, error)
}
