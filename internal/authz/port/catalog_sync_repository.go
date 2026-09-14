package authzport

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/authz/domain/catalogsync"
)

// CatalogSyncRepository stores where catalogs come from and what happened
// when they were read. The run history is written OUTSIDE the apply
// transaction on purpose: a failed apply rolls back its rows, and the record
// that it was attempted must survive that rollback or a feed can fail
// silently forever.
type CatalogSyncRepository interface {
	ListSources(ctx context.Context, tenantID string) ([]catalogsync.Source, error)
	Source(ctx context.Context, tenantID, id string) (*catalogsync.Source, error)
	// DueSources crosses tenants: the scheduler is one loop for the
	// installation, not one per tenant.
	DueSources(ctx context.Context, now time.Time, limit int32) ([]catalogsync.Source, error)
	CreateSource(ctx context.Context, s catalogsync.Source) (string, error)
	UpdateSource(ctx context.Context, s catalogsync.Source) error
	// DeleteSource removes a source and its history. The applies it made
	// survive in the audit log.
	DeleteSource(ctx context.Context, tenantID, id string) error
	// ScheduleNext moves a source past the run that just happened.
	ScheduleNext(ctx context.Context, sourceID string) error

	StartRun(ctx context.Context, sourceID, tenantID, actor string, dry bool) (string, error)
	FinishRun(ctx context.Context, runID, status, digest, reportJSON, errMsg string) error
	ListRuns(ctx context.Context, sourceID string, limit int32) ([]catalogsync.Run, error)
	// LastAppliedDigest is what makes an unchanged catalog cost nothing.
	LastAppliedDigest(ctx context.Context, sourceID string) (string, error)
}
