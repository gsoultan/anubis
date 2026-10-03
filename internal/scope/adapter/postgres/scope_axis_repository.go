package scopepg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/scope/adapter/postgres/rgen/scopeaxis"
	scopermquery "github.com/gsoultan/anubis/internal/scope/adapter/postgres/rquery"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/storm/runtime"
)

// ListScopeAxes is one tenant's structures (0056).
func (s *Repository) ListScopeAxes(ctx context.Context, tenantID string) ([]scopedomain.ScopeAxisRecord, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	rows, err := scopeaxis.New().
		Where(scopeaxis.TenantID.Eq(tid)).
		Order(scopeaxis.SortOrder.Asc(), scopeaxis.Code.Asc()).
		All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]scopedomain.ScopeAxisRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, axisRecord(r))
	}
	return out, nil
}

func (s *Repository) ScopeAxis(ctx context.Context, tenantID, code string) (*scopedomain.ScopeAxisRecord, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	r, ok, err := scopeaxis.New().Where(scopeaxis.TenantID.Eq(tid), scopeaxis.Code.Eq(code)).One(ctx, s.ex(ctx))
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, apperr.ErrNotFound.With("scope_axis", code)
	}
	rec := axisRecord(r)
	return &rec, nil
}

func axisRecord(r scopeaxis.Row) scopedomain.ScopeAxisRecord {
	return scopedomain.ScopeAxisRecord{
		Code: r.Code, DisplayName: r.DisplayName, DefaultEffect: r.DefaultEffect,
		Status: r.Status, SortOrder: int(r.SortOrder),
		Resolution: []byte(r.Resolution), UISchema: []byte(r.UiSchema),
	}
}

func (s *Repository) CreateScopeAxis(ctx context.Context, tenantID string, a scopedomain.ScopeAxisRecord) error {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	n := scopeaxis.Create()
	n.SetTenantID(tid)
	n.SetCode(a.Code)
	n.SetDisplayName(a.DisplayName)
	n.SetDefaultEffect(database.OrDefaultStr(a.DefaultEffect, "unconstrained"))
	n.SetSortOrder(int32(database.OrDefaultInt(a.SortOrder, 100)))
	n.SetResolution(runtime.JSON(database.OrDefaultJSON(a.Resolution, `{"from":"context"}`)))
	n.SetUiSchema(runtime.JSON(database.OrEmptyJSON(a.UISchema)))
	_, err = n.Insert(ctx, s.ex(ctx))
	return database.MapErr(err)
}

func (s *Repository) UpdateScopeAxis(ctx context.Context, tenantID string, a scopedomain.ScopeAxisRecord) error {
	n, err := scopermquery.UpdateScopeAxis.Exec(ctx, s.ex(ctx),
		a.Code, a.DisplayName, a.DefaultEffect, a.Status,
		int32(a.SortOrder), database.OrEmptyJSON(a.UISchema), tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		// Not this tenant's structure — or none at all. One answer for both.
		return apperr.ErrNotFound.With("scope_axis", a.Code)
	}
	return nil
}
