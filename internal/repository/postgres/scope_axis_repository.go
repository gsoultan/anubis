package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) ListScopeAxes(ctx context.Context) ([]repository.ScopeAxisRecord, error) {
	rows, err := s.q(ctx).ListScopeAxes(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.ScopeAxisRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.ScopeAxisRecord{
			Code: r.Code, DisplayName: r.DisplayName, DefaultEffect: r.DefaultEffect,
			Status: r.Status, SortOrder: int(r.SortOrder),
			Resolution: r.Resolution, UISchema: r.UiSchema,
		})
	}
	return out, nil
}

func (s *Store) ScopeAxis(ctx context.Context, code string) (*repository.ScopeAxisRecord, error) {
	r, err := s.q(ctx).GetScopeAxis(ctx, code)
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.ScopeAxisRecord{
		Code: r.Code, DisplayName: r.DisplayName, DefaultEffect: r.DefaultEffect,
		Status: r.Status, SortOrder: int(r.SortOrder),
		Resolution: r.Resolution, UISchema: r.UiSchema,
	}, nil
}

func (s *Store) CreateScopeAxis(ctx context.Context, a repository.ScopeAxisRecord) error {
	_, err := s.q(ctx).CreateScopeAxis(ctx, gen.CreateScopeAxisParams{
		Code: a.Code, DisplayName: a.DisplayName,
		DefaultEffect: orDefaultStr(a.DefaultEffect, "unconstrained"),
		SortOrder:     int32(orDefaultInt(a.SortOrder, 100)),
		Resolution:    orDefaultJSON(a.Resolution, `{"from":"context"}`),
		UiSchema:      orEmptyJSON(a.UISchema),
	})
	return mapErr(err)
}

func (s *Store) UpdateScopeAxis(ctx context.Context, a repository.ScopeAxisRecord) error {
	_, err := s.q(ctx).UpdateScopeAxis(ctx, gen.UpdateScopeAxisParams{
		Code: a.Code, DisplayName: a.DisplayName, DefaultEffect: a.DefaultEffect,
		Status: a.Status, SortOrder: int32(a.SortOrder), UiSchema: orEmptyJSON(a.UISchema),
	})
	return mapErr(err)
}

func orDefaultInt(v, d int) int {
	if v == 0 {
		return d
	}
	return v
}

func orDefaultJSON(b []byte, d string) []byte {
	if len(b) == 0 {
		return []byte(d)
	}
	return b
}
