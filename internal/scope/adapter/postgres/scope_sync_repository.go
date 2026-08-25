package scopepg

import (
	"context"

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
			ID: r.ID, Axis: r.AxisCode, Kind: r.Kind, Status: r.Status,
			Config: r.Config, LastRunAt: r.LastRunAt,
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
		ID: r.ID, Axis: r.AxisCode, Kind: r.Kind, Status: r.Status,
		Config: r.Config, LastRunAt: r.LastRunAt,
	}, nil
}

func (s *Repository) CreateSyncSource(ctx context.Context, tenantID string, src scopedomain.SyncSourceRecord) (string, error) {
	id, err := s.q(ctx).CreateSyncSource(ctx, gen.CreateSyncSourceParams{
		TenantID: tenantID, AxisCode: src.Axis, Kind: src.Kind,
		Config: database.OrEmptyJSON(src.Config),
	})
	return id, database.MapErr(err)
}

func (s *Repository) UpdateSyncSource(ctx context.Context, tenantID string, src scopedomain.SyncSourceRecord) error {
	n, err := s.q(ctx).UpdateSyncSource(ctx, gen.UpdateSyncSourceParams{
		ID: src.ID, TenantID: tenantID,
		Config: database.OrEmptyJSON(src.Config),
		Status: database.OrDefaultStr(src.Status, "active"),
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
