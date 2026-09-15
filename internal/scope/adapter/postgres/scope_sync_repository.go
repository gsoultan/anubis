package scopepg

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/platform/database"
	gen "github.com/gsoultan/anubis/internal/scope/adapter/postgres/gen"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

func (s *Repository) ListSyncSources(ctx context.Context, tenantID string) ([]scopedomain.SyncSourceRecord, error) {
	rows, err := s.q(ctx).ListSyncSources(ctx, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]scopedomain.SyncSourceRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, scopedomain.SyncSourceRecord{
			ID: r.ID, TenantID: r.TenantID, Axis: r.AxisCode, Kind: r.Kind,
			Status: r.Status, Config: r.Config, LastRunAt: r.LastRunAt,
			IntervalSeconds: r.IntervalSeconds, NextRunAt: r.NextRunAt,
		})
	}
	return out, nil
}

func (s *Repository) SyncSource(ctx context.Context, tenantID, id string) (*scopedomain.SyncSourceRecord, error) {
	r, err := s.q(ctx).GetSyncSource(ctx, gen.GetSyncSourceParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &scopedomain.SyncSourceRecord{
		ID: r.ID, TenantID: r.TenantID, Axis: r.AxisCode, Kind: r.Kind,
		Status: r.Status, Config: r.Config, LastRunAt: r.LastRunAt,
		IntervalSeconds: r.IntervalSeconds, NextRunAt: r.NextRunAt,
	}, nil
}

func (s *Repository) CreateSyncSource(ctx context.Context, tenantID string, src scopedomain.SyncSourceRecord) (string, error) {
	id, err := s.q(ctx).CreateSyncSource(ctx, gen.CreateSyncSourceParams{
		TenantID: tenantID, AxisCode: src.Axis, Kind: src.Kind,
		Config: database.OrEmptyJSON(src.Config), IntervalSeconds: src.IntervalSeconds,
	})
	return id, database.MapErr(err)
}

func (s *Repository) UpdateSyncSource(ctx context.Context, tenantID string, src scopedomain.SyncSourceRecord) error {
	n, err := s.q(ctx).UpdateSyncSource(ctx, gen.UpdateSyncSourceParams{
		ID: src.ID, TenantID: tenantID,
		Config:          database.OrEmptyJSON(src.Config),
		Status:          database.OrDefaultStr(src.Status, "active"),
		IntervalSeconds: src.IntervalSeconds,
	})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return database.NotFound()
	}
	return nil
}

func (s *Repository) ScopeSyncApply(ctx context.Context, sourceID string, rows []byte, dry bool) (string, error) {
	report, err := s.q(ctx).ScopeSyncApply(ctx, gen.ScopeSyncApplyParams{
		SourceID: sourceID, Rows: rows, Dry: dry,
	})
	return report, database.MapErr(err)
}

// ListSyncRuns reads the history scope_sync_apply has been writing since
// migration 0017. Tenant scoping is in the query's join, not in the
// caller's care: the runs table carries no tenant_id of its own.
func (s *Repository) ListSyncRuns(ctx context.Context, tenantID, sourceID string, limit int32) ([]scopedomain.SyncRun, error) {
	rows, err := s.q(ctx).ListSyncRuns(ctx, gen.ListSyncRunsParams{
		TenantID: tenantID, SourceID: sourceID, Lim: limit,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]scopedomain.SyncRun, 0, len(rows))
	for _, r := range rows {
		out = append(out, scopedomain.SyncRun{
			ID: r.ID, SourceID: r.SourceID, AxisCode: r.AxisCode,
			StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
			Dry: r.Dry, Status: r.Status, Report: r.Report,
		})
	}
	return out, nil
}

// DueSyncSources spans every tenant on purpose — one timer serves them all —
// which is exactly why the usecase above it is kept off the transport. The
// partial index added in 0045 means a tick that finds nothing costs an index
// probe rather than a scan of every source in the installation.
func (s *Repository) DueSyncSources(ctx context.Context, now time.Time, limit int32) ([]scopedomain.SyncSourceRecord, error) {
	rows, err := s.q(ctx).DueSyncSources(ctx, gen.DueSyncSourcesParams{Now: &now, Lim: limit})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]scopedomain.SyncSourceRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, scopedomain.SyncSourceRecord{
			ID: r.ID, TenantID: r.TenantID, Axis: r.AxisCode, Kind: r.Kind,
			Status: r.Status, Config: r.Config, LastRunAt: r.LastRunAt,
			IntervalSeconds: r.IntervalSeconds, NextRunAt: r.NextRunAt,
		})
	}
	return out, nil
}

func (s *Repository) RecordSyncFailure(ctx context.Context, sourceID, reason string) error {
	return database.MapErr(s.q(ctx).RecordSyncFailure(ctx, gen.RecordSyncFailureParams{
		SourceID: sourceID, Reason: reason,
	}))
}

func (s *Repository) SetSyncSchedule(ctx context.Context, tenantID, id string, intervalSeconds int32) error {
	n, err := s.q(ctx).SetSyncSchedule(ctx, gen.SetSyncScheduleParams{
		ID: id, TenantID: tenantID, IntervalSeconds: intervalSeconds,
	})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return database.NotFound()
	}
	return nil
}

func (s *Repository) ScheduleNextSyncSource(ctx context.Context, sourceID string) error {
	return database.MapErr(s.q(ctx).ScheduleNextSyncSource(ctx, sourceID))
}
