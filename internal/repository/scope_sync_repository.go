package repository

import "context"

type ScopeSyncRepository interface {
	ListSyncSources(ctx context.Context, tenantID string) ([]SyncSourceRecord, error)
	SyncSource(ctx context.Context, tenantID, id string) (*SyncSourceRecord, error)
	CreateSyncSource(ctx context.Context, tenantID string, s SyncSourceRecord) (string, error)
	ScopeSyncApply(ctx context.Context, sourceID string, rows []byte, dry bool) (string, error)
}
