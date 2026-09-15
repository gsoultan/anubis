package scopeport

import (
	"context"
	"time"

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
	// DueSyncSources is the scheduler's read and spans every tenant, because a
	// timer serves them all. Each record carries its own TenantID precisely
	// because there is no ambient one to fall back on.
	DueSyncSources(ctx context.Context, now time.Time, limit int32) ([]scopedomain.SyncSourceRecord, error)
	// RecordSyncFailure records an attempt that failed before the reconciler
	// ran, which is the only kind scope_sync_apply cannot record for itself.
	RecordSyncFailure(ctx context.Context, sourceID, reason string) error
	// SetSyncSchedule changes when a source runs, touching neither its config
	// nor its status — the console cannot safely round-trip a config it is
	// never sent the secrets of.
	SetSyncSchedule(ctx context.Context, tenantID, id string, intervalSeconds int32) error
	// ScheduleNextSyncSource moves a source on whether or not its run worked.
	ScheduleNextSyncSource(ctx context.Context, sourceID string) error
}
