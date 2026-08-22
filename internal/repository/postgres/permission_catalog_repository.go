package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) ListPermissions(ctx context.Context, tenantID, applicationID string, includeDeprecated bool) ([]repository.PermissionRecord, error) {
	rows, err := s.q(ctx).ListPermissions(ctx, gen.ListPermissionsParams{
		TenantID: tenantID, ApplicationID: optStr(applicationID),
		IncludeDeprecated: includeDeprecated,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.PermissionRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.PermissionRecord{
			ID: r.ID, Key: deref(r.Key), AppSlug: r.AppSlug,
			Resource: r.Resource, Action: r.Action, Risk: r.Risk,
			Description: r.Description, MinAssurance: int(r.MinAssurance),
			RequiresAMR: r.RequiresAmr, MaxAuthAge: r.MaxAuthAge,
			Deprecated: r.DeprecatedAt != nil,
		})
	}
	return out, nil
}

func (s *Store) UpsertPermission(ctx context.Context, tenantID, applicationID, appSlug string, p repository.PermissionRecord) (string, string, error) {
	row, err := s.q(ctx).UpsertPermission(ctx, gen.UpsertPermissionParams{
		ApplicationID: applicationID, TenantID: tenantID, AppSlug: appSlug,
		Resource: p.Resource, Action: p.Action, Description: p.Description,
		Risk:         orDefaultStr(p.Risk, "normal"),
		MinAssurance: int16(maxInt(p.MinAssurance, 1)),
		RequiresAmr:  emptyIfNil(p.RequiresAMR),
		MaxAuthAge:   p.MaxAuthAge,
	})
	if err != nil {
		return "", "", mapErr(err)
	}
	return row.ID, deref(row.Key), nil
}

func (s *Store) DeprecatePermissionsExcept(ctx context.Context, applicationID string, keepIDs []string) ([]string, error) {
	keys, err := s.q(ctx).DeprecatePermissionsExcept(ctx, gen.DeprecatePermissionsExceptParams{
		ApplicationID: applicationID, KeepIds: keepIDs,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != nil {
			out = append(out, *k)
		}
	}
	return out, nil
}

func (s *Store) PermissionIDByKey(ctx context.Context, tenantID, key string) (string, error) {
	id, err := s.q(ctx).PermissionIDByKey(ctx, gen.PermissionIDByKeyParams{
		TenantID: tenantID, Key: optStr(key),
	})
	return id, mapErr(err)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
