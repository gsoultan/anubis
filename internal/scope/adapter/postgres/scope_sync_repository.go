package scopepg

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/scope/adapter/postgres/rgen/scopesyncsource"
	scopermquery "github.com/gsoultan/anubis/internal/scope/adapter/postgres/rquery"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

func (s *Repository) ListSyncSources(ctx context.Context, tenantID string) ([]scopedomain.SyncSourceRecord, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	rows, err := scopesyncsource.New().
		Where(scopesyncsource.TenantID.Eq(tid)).
		Order(scopesyncsource.AxisCode.Asc()).
		All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return sourceRecords(rows), nil
}

func (s *Repository) SyncSource(ctx context.Context, tenantID, id string) (*scopedomain.SyncSourceRecord, error) {
	tid, sid, err := twoUUIDs(tenantID, id)
	if err != nil {
		return nil, err
	}
	r, ok, err := scopesyncsource.New().
		Where(scopesyncsource.ID.Eq(sid), scopesyncsource.TenantID.Eq(tid)).
		One(ctx, s.ex(ctx))
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	rec := sourceRecord(r)
	return &rec, nil
}

// DueSyncSources is the scheduler's only read. It carries no tenant filter
// because a timer serves every tenant at once; that is exactly why the usecase
// behind it must never be reachable from a transport.
func (s *Repository) DueSyncSources(ctx context.Context, now time.Time, limit int32) ([]scopedomain.SyncSourceRecord, error) {
	rows, err := scopesyncsource.New().
		Where(scopesyncsource.Status.Eq("active"),
			scopesyncsource.NextRunAt.IsNotNull(),
			scopesyncsource.NextRunAt.Lte(now)).
		Order(scopesyncsource.NextRunAt.Asc()).
		Limit(int64(limit)).
		All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return sourceRecords(rows), nil
}

func sourceRecords(rows []scopesyncsource.Row) []scopedomain.SyncSourceRecord {
	out := make([]scopedomain.SyncSourceRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, sourceRecord(r))
	}
	return out
}

func sourceRecord(r scopesyncsource.Row) scopedomain.SyncSourceRecord {
	rec := scopedomain.SyncSourceRecord{
		ID: database.UUIDStr(r.ID), TenantID: database.UUIDStr(r.TenantID),
		Axis: r.AxisCode, Kind: r.Kind, Status: r.Status,
		Config: []byte(r.Config), IntervalSeconds: r.IntervalSeconds,
	}
	if v, ok := r.LastRunAt.Get(); ok {
		rec.LastRunAt = &v
	}
	if v, ok := r.NextRunAt.Get(); ok {
		rec.NextRunAt = &v
	}
	return rec
}

func twoUUIDs(a, b string) ([16]byte, [16]byte, error) {
	x, err := database.ParseUUID(a)
	if err != nil {
		return x, x, database.MapErr(err)
	}
	y, err := database.ParseUUID(b)
	if err != nil {
		return x, y, database.MapErr(err)
	}
	return x, y, nil
}

func (s *Repository) CreateSyncSource(ctx context.Context, tenantID string, src scopedomain.SyncSourceRecord) (string, error) {
	row, _, err := scopermquery.CreateSyncSource.One(ctx, s.ex(ctx),
		tenantID, src.Axis, src.Kind,
		database.OrEmptyJSON(src.Config), src.IntervalSeconds)
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.ID, nil
}

// UpdateSyncSource replaces the configuration wholesale — merging secrets is
// how half-rotated credentials happen — and reschedules only if the interval
// actually changed.
func (s *Repository) UpdateSyncSource(ctx context.Context, tenantID string, src scopedomain.SyncSourceRecord) error {
	n, err := scopermquery.UpdateSyncSource.Exec(ctx, s.ex(ctx),
		src.ID, tenantID, database.OrEmptyJSON(src.Config),
		database.OrDefaultStr(src.Status, "active"), src.IntervalSeconds)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return database.NotFound()
	}
	return nil
}

// SetSyncSchedule changes WHEN a source runs and nothing else: the console is
// never sent dsn or auth_header, so rescheduling through UpdateSyncSource
// would save a config with the credentials missing.
func (s *Repository) SetSyncSchedule(ctx context.Context, tenantID, id string, intervalSeconds int32) error {
	n, err := scopermquery.SetSyncSchedule.Exec(ctx, s.ex(ctx), id, tenantID, intervalSeconds)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return database.NotFound()
	}
	return nil
}

func (s *Repository) ScheduleNextSyncSource(ctx context.Context, sourceID string) error {
	_, err := scopermquery.ScheduleNextSyncSource.Exec(ctx, s.ex(ctx), sourceID)
	return database.MapErr(err)
}

func (s *Repository) RecordSyncFailure(ctx context.Context, sourceID, reason string) error {
	_, err := scopermquery.RecordSyncFailure.Exec(ctx, s.ex(ctx), sourceID, reason)
	return database.MapErr(err)
}

// ScopeSyncApply reconciles a whole feed in one statement; the function opens
// its own run row and catches per-row errors.
func (s *Repository) ScopeSyncApply(ctx context.Context, sourceID string, rows []byte, dry bool) (string, error) {
	row, _, err := scopermquery.ScopeSyncApply.One(ctx, s.ex(ctx),
		sourceID, database.OrEmptyJSON(rows), dry)
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.Report, nil
}

func (s *Repository) ListSyncRuns(ctx context.Context, tenantID, sourceID string, limit int32) ([]scopedomain.SyncRun, error) {
	rows, err := scopermquery.ListSyncRuns.Query(ctx, s.ex(ctx), tenantID, sourceID, limit)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]scopedomain.SyncRun, 0, len(rows))
	for _, r := range rows {
		run := scopedomain.SyncRun{
			ID: r.ID, SourceID: r.SourceID, AxisCode: r.AxisCode,
			StartedAt: r.StartedAt, Dry: r.Dry, Status: r.Status,
			Report: []byte(r.Report),
		}
		if v, ok := r.FinishedAt.Get(); ok {
			run.FinishedAt = &v
		}
		out = append(out, run)
	}
	return out, nil
}
