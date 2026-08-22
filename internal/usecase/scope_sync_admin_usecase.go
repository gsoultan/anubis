package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/repository"
)

type ScopeSyncAdminUsecase interface {
	ListSyncSources(ctx context.Context) ([]repository.SyncSourceRecord, error)
	CreateSyncSource(ctx context.Context, s repository.SyncSourceRecord) (*repository.SyncSourceRecord, error)
	RunSync(ctx context.Context, sourceID string, rows []SyncRowInput, dry bool) (reportJSON string, err error)
}
