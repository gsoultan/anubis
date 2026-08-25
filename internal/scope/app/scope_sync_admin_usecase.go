package scopeapp

import (
	"context"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

type ScopeSyncAdminUsecase interface {
	ListSyncSources(ctx context.Context) ([]scopedomain.SyncSourceRecord, error)
	CreateSyncSource(ctx context.Context, s scopedomain.SyncSourceRecord) (*scopedomain.SyncSourceRecord, error)
	UpdateSyncSource(ctx context.Context, s scopedomain.SyncSourceRecord) (*scopedomain.SyncSourceRecord, error)
	RunSync(ctx context.Context, sourceID string, rows []SyncRowInput, dry bool) (reportJSON string, err error)
	// ListSyncRuns is what a feed has actually done, newest first. The
	// database has recorded this since 0017; nothing read it until now.
	ListSyncRuns(ctx context.Context, sourceID string, limit int32) ([]scopedomain.SyncRun, error)
}
