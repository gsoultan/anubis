package scopepg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
	gen "github.com/gsoultan/anubis/internal/scope/adapter/postgres/gen"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

func (s *Repository) ListScopeAxes(ctx context.Context) ([]scopedomain.ScopeAxisRecord, error) {
	rows, err := s.q(ctx).ListScopeAxes(ctx)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]scopedomain.ScopeAxisRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, scopedomain.ScopeAxisRecord{
			Code: r.Code, DisplayName: r.DisplayName, DefaultEffect: r.DefaultEffect,
			Status: r.Status, SortOrder: int(r.SortOrder),
			Resolution: r.Resolution, UISchema: r.UiSchema,
		})
	}
	return out, nil
}

func (s *Repository) ScopeAxis(ctx context.Context, code string) (*scopedomain.ScopeAxisRecord, error) {
	r, err := s.q(ctx).GetScopeAxis(ctx, code)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &scopedomain.ScopeAxisRecord{
		Code: r.Code, DisplayName: r.DisplayName, DefaultEffect: r.DefaultEffect,
		Status: r.Status, SortOrder: int(r.SortOrder),
		Resolution: r.Resolution, UISchema: r.UiSchema,
	}, nil
}

func (s *Repository) CreateScopeAxis(ctx context.Context, a scopedomain.ScopeAxisRecord) error {
	_, err := s.q(ctx).CreateScopeAxis(ctx, gen.CreateScopeAxisParams{
		Code: a.Code, DisplayName: a.DisplayName,
		DefaultEffect: database.OrDefaultStr(a.DefaultEffect, "unconstrained"),
		SortOrder:     int32(database.OrDefaultInt(a.SortOrder, 100)),
		Resolution:    database.OrDefaultJSON(a.Resolution, `{"from":"context"}`),
		UiSchema:      database.OrEmptyJSON(a.UISchema),
	})
	return database.MapErr(err)
}

func (s *Repository) UpdateScopeAxis(ctx context.Context, a scopedomain.ScopeAxisRecord) error {
	_, err := s.q(ctx).UpdateScopeAxis(ctx, gen.UpdateScopeAxisParams{
		Code: a.Code, DisplayName: a.DisplayName, DefaultEffect: a.DefaultEffect,
		Status: a.Status, SortOrder: int32(a.SortOrder), UiSchema: database.OrEmptyJSON(a.UISchema),
	})
	return database.MapErr(err)
}
