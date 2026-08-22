package authzpg

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/authz/adapter/postgres/gen"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) ListPermissions(ctx context.Context, tenantID, applicationID string, includeDeprecated bool) ([]authzdomain.PermissionRecord, error) {
	rows, err := s.q(ctx).ListPermissions(ctx, gen.ListPermissionsParams{
		TenantID: tenantID, ApplicationID: database.OptStr(applicationID),
		IncludeDeprecated: includeDeprecated,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authzdomain.PermissionRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, authzdomain.PermissionRecord{
			ID: r.ID, Key: database.Deref(r.Key), AppSlug: r.AppSlug,
			Resource: r.Resource, Action: r.Action, Risk: r.Risk,
			Description: r.Description, MinAssurance: int(r.MinAssurance),
			RequiresAMR: r.RequiresAmr, MaxAuthAge: r.MaxAuthAge,
			Deprecated: r.DeprecatedAt != nil,
		})
	}
	return out, nil
}

func (s *Repository) UpsertPermission(ctx context.Context, tenantID, applicationID, appSlug string, p authzdomain.PermissionRecord) (string, string, error) {
	row, err := s.q(ctx).UpsertPermission(ctx, gen.UpsertPermissionParams{
		ApplicationID: applicationID, TenantID: tenantID, AppSlug: appSlug,
		Resource: p.Resource, Action: p.Action, Description: p.Description,
		Risk:         database.OrDefaultStr(p.Risk, "normal"),
		MinAssurance: int16(database.MaxInt(p.MinAssurance, 1)),
		RequiresAmr:  database.EmptyIfNil(p.RequiresAMR),
		MaxAuthAge:   p.MaxAuthAge,
	})
	if err != nil {
		return "", "", database.MapErr(err)
	}
	return row.ID, database.Deref(row.Key), nil
}

func (s *Repository) DeprecatePermissionsExcept(ctx context.Context, applicationID string, keepIDs []string) ([]string, error) {
	keys, err := s.q(ctx).DeprecatePermissionsExcept(ctx, gen.DeprecatePermissionsExceptParams{
		ApplicationID: applicationID, KeepIds: keepIDs,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != nil {
			out = append(out, *k)
		}
	}
	return out, nil
}

func (s *Repository) PermissionIDByKey(ctx context.Context, tenantID, key string) (string, error) {
	id, err := s.q(ctx).PermissionIDByKey(ctx, gen.PermissionIDByKeyParams{
		TenantID: tenantID, Key: database.OptStr(key),
	})
	return id, database.MapErr(err)
}
