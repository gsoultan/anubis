package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) ListSyncSources(ctx context.Context, tenantID string) ([]repository.SyncSourceRecord, error) {
	rows, err := s.q(ctx).ListSyncSources(ctx, tenantID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.SyncSourceRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.SyncSourceRecord{
			ID: r.ID, Axis: r.AxisCode, Kind: r.Kind, Status: r.Status,
			Config: r.Config, LastRunAt: r.LastRunAt,
		})
	}
	return out, nil
}

func (s *Store) SyncSource(ctx context.Context, tenantID, id string) (*repository.SyncSourceRecord, error) {
	r, err := s.q(ctx).GetSyncSource(ctx, gen.GetSyncSourceParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.SyncSourceRecord{
		ID: r.ID, Axis: r.AxisCode, Kind: r.Kind, Status: r.Status,
		Config: r.Config, LastRunAt: r.LastRunAt,
	}, nil
}

func (s *Store) CreateSyncSource(ctx context.Context, tenantID string, src repository.SyncSourceRecord) (string, error) {
	id, err := s.q(ctx).CreateSyncSource(ctx, gen.CreateSyncSourceParams{
		TenantID: tenantID, AxisCode: src.Axis, Kind: src.Kind,
		Config: orEmptyJSON(src.Config),
	})
	return id, mapErr(err)
}

func (s *Store) UpdateSyncSource(ctx context.Context, tenantID string, src repository.SyncSourceRecord) error {
	n, err := s.q(ctx).UpdateSyncSource(ctx, gen.UpdateSyncSourceParams{
		ID: src.ID, TenantID: tenantID,
		Config: orEmptyJSON(src.Config),
		Status: orDefaultStr(src.Status, "active"),
	})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return notFoundErr()
	}
	return nil
}

func (s *Store) ScopeSyncApply(ctx context.Context, sourceID string, rows []byte, dry bool) (string, error) {
	report, err := s.q(ctx).ScopeSyncApply(ctx, gen.ScopeSyncApplyParams{
		SourceID: sourceID, Rows: rows, Dry: dry,
	})
	return report, mapErr(err)
}
