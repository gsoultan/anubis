package authzpg

import (
	"context"

	authzrquery "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rquery"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) ListPermissions(ctx context.Context, tenantID, applicationID string, includeDeprecated bool) ([]authzdomain.PermissionRecord, error) {
	rows, err := authzrquery.ListPermissions.Query(ctx, s.rex(ctx),
		tenantID, database.OptStr(applicationID), includeDeprecated)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authzdomain.PermissionRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, authzdomain.PermissionRecord{
			ID: r.ID, Key: r.Key.V, AppSlug: r.AppSlug,
			Resource: r.Resource, Action: r.Action, Risk: r.Risk,
			Description: r.Description, MinAssurance: int(r.MinAssurance),
			RequiresAMR: r.RequiresAmr, MaxAuthAge: r.MaxAuthAge,
			Deprecated: r.DeprecatedAt.Valid,
		})
	}
	return out, nil
}

func (s *Repository) UpsertPermission(ctx context.Context, tenantID, applicationID, appSlug string, p authzdomain.PermissionRecord) (string, string, error) {
	row, ok, err := authzrquery.UpsertPermission.One(ctx, s.rex(ctx),
		applicationID, tenantID, appSlug, p.Resource, p.Action, p.Description,
		database.OrDefaultStr(p.Risk, "normal"),
		int16(database.MaxInt(p.MinAssurance, 1)),
		database.EmptyIfNil(p.RequiresAMR), p.MaxAuthAge)
	if err != nil {
		return "", "", database.MapErr(err)
	}
	if !ok {
		return "", "", apperr.ErrNotFound
	}
	return row.ID, row.Key.V, nil
}

func (s *Repository) DeprecatePermissionsExcept(ctx context.Context, applicationID string, keepIDs []string) ([]string, error) {
	rows, err := authzrquery.DeprecatePermissionsExcept.Query(ctx, s.rex(ctx), applicationID, keepIDs)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Key.Valid {
			out = append(out, r.Key.V)
		}
	}
	return out, nil
}

func (s *Repository) PermissionIDByKey(ctx context.Context, tenantID, key string) (string, error) {
	row, ok, err := authzrquery.PermissionIDByKey.One(ctx, s.rex(ctx),
		tenantID, database.OptStr(key))
	if err != nil {
		return "", database.MapErr(err)
	}
	if !ok {
		return "", apperr.ErrNotFound
	}
	return row.ID, nil
}
