package authzpg

import (
	"context"
	"time"

	authzrquery "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rquery"
	"github.com/gsoultan/anubis/internal/authz/domain/catalogsync"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func sourceFrom(r authzrquery.CatalogSourceRow) catalogsync.Source {
	s := catalogsync.Source{
		ID: r.ID, TenantID: r.TenantID, ApplicationID: r.ApplicationID,
		ApplicationSlug: r.ApplicationSlug, Kind: r.Kind, Format: r.Format,
		Status: r.Status, Name: r.Name, Config: []byte(r.Config),
		IntervalSeconds: int(r.IntervalSeconds), LastStatus: r.LastStatus.V,
	}
	if r.LastRunAt.Valid {
		t := r.LastRunAt.V
		s.LastRunAt = &t
	}
	if r.NextRunAt.Valid {
		t := r.NextRunAt.V
		s.NextRunAt = &t
	}
	return s
}

func (s *Repository) ListSources(ctx context.Context, tenantID string) ([]catalogsync.Source, error) {
	rows, err := authzrquery.ListCatalogSources.Query(ctx, s.rex(ctx), tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]catalogsync.Source, 0, len(rows))
	for _, r := range rows {
		out = append(out, sourceFrom(r))
	}
	return out, nil
}

func (s *Repository) Source(ctx context.Context, tenantID, id string) (*catalogsync.Source, error) {
	row, ok, err := authzrquery.GetCatalogSource.One(ctx, s.rex(ctx), id, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, apperr.ErrNotFound.With("source", id)
	}
	rec := sourceFrom(row)
	return &rec, nil
}

func (s *Repository) DueSources(ctx context.Context, now time.Time, limit int32) ([]catalogsync.Source, error) {
	rows, err := authzrquery.DueCatalogSources.Query(ctx, s.rex(ctx), now, limit)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]catalogsync.Source, 0, len(rows))
	for _, r := range rows {
		out = append(out, sourceFrom(r))
	}
	return out, nil
}

func (s *Repository) CreateSource(ctx context.Context, src catalogsync.Source) (string, error) {
	row, ok, err := authzrquery.CreateCatalogSource.One(ctx, s.rex(ctx),
		src.TenantID, src.ApplicationID, src.Kind, src.Format, src.Status,
		src.Name, string(src.Config), int32(src.IntervalSeconds))
	if err != nil {
		return "", database.MapErr(err)
	}
	if !ok {
		return "", apperr.ErrInternal
	}
	return row.ID, nil
}

func (s *Repository) UpdateSource(ctx context.Context, src catalogsync.Source) error {
	n, err := authzrquery.UpdateCatalogSource.Exec(ctx, s.rex(ctx),
		src.ID, src.TenantID, src.Name, src.Status, string(src.Config),
		src.Format, int32(src.IntervalSeconds))
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound.With("source", src.ID)
	}
	return nil
}

func (s *Repository) DeleteSource(ctx context.Context, tenantID, id string) error {
	n, err := authzrquery.DeleteCatalogSource.Exec(ctx, s.rex(ctx), id, tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound.With("source", id)
	}
	return nil
}

func (s *Repository) ScheduleNext(ctx context.Context, sourceID string) error {
	if _, err := authzrquery.ScheduleCatalogSource.Exec(ctx, s.rex(ctx), sourceID); err != nil {
		return database.MapErr(err)
	}
	return nil
}

func (s *Repository) StartRun(ctx context.Context, sourceID, tenantID, actor string, dry bool) (string, error) {
	row, ok, err := authzrquery.StartCatalogRun.One(ctx, s.rex(ctx), sourceID, tenantID, dry, actor)
	if err != nil {
		return "", database.MapErr(err)
	}
	if !ok {
		return "", apperr.ErrInternal
	}
	return row.ID, nil
}

func (s *Repository) FinishRun(ctx context.Context, runID, status, digest, reportJSON, errMsg string) error {
	if _, err := authzrquery.FinishCatalogRun.Exec(ctx, s.rex(ctx),
		runID, status, digest, reportJSON, errMsg); err != nil {
		return database.MapErr(err)
	}
	return nil
}

func (s *Repository) ListRuns(ctx context.Context, sourceID string, limit int32) ([]catalogsync.Run, error) {
	rows, err := authzrquery.ListCatalogRuns.Query(ctx, s.rex(ctx), sourceID, limit)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]catalogsync.Run, 0, len(rows))
	for _, r := range rows {
		run := catalogsync.Run{
			ID: r.ID, SourceID: r.SourceID, StartedAt: r.StartedAt, Dry: r.Dry,
			Status: r.Status, Actor: r.Actor, DocumentSHA: r.DocumentSha,
			Error: r.Error,
		}
		if r.FinishedAt.Valid {
			t := r.FinishedAt.V
			run.FinishedAt = &t
		}
		if r.Report.Valid {
			run.Report = r.Report.V
		}
		out = append(out, run)
	}
	return out, nil
}

func (s *Repository) LastAppliedDigest(ctx context.Context, sourceID string) (string, error) {
	row, ok, err := authzrquery.LastAppliedDigest.One(ctx, s.rex(ctx), sourceID)
	if err != nil {
		return "", database.MapErr(err)
	}
	if !ok {
		return "", nil
	}
	return row.DocumentSha, nil
}
