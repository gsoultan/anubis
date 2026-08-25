package authzpg

import (
	"context"

	rrole "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rgen/role"
	authzrquery "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rquery"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func roleRecordOf(r authzrquery.RoleRow) authzdomain.RoleRecord {
	return authzdomain.RoleRecord{
		ID: r.ID, Name: r.Name, Description: r.Description,
		ApplicationSlug: r.ApplicationSlug.V, IsSystem: r.IsSystem,
		AllowedRealmKinds: r.AllowedRealmKinds, AssignableAt: r.AssignableAt,
	}
}

func (s *Repository) ListRoles(ctx context.Context, tenantID, query string) ([]authzdomain.RoleRecord, error) {
	rows, err := authzrquery.ListRoles.Query(ctx, s.rex(ctx), tenantID, database.OptStr(query))
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authzdomain.RoleRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, roleRecordOf(r))
	}
	return out, nil
}

func (s *Repository) RoleByID(ctx context.Context, tenantID, id string) (*authzdomain.RoleRecord, error) {
	r, ok, err := authzrquery.GetRole.One(ctx, s.rex(ctx), id, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, apperr.ErrNotFound
	}
	rec := roleRecordOf(r)
	return &rec, nil
}

// RoleByName is the migration's builder query: no SQL anywhere, the predicate
// set compiled from the rmodel projection.
func (s *Repository) RoleByName(ctx context.Context, tenantID, name string) (*authzdomain.RoleRecord, error) {
	tid, err := parseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	r, ok, err := rrole.New().
		Where(rrole.TenantID.Eq(tid), rrole.Name.Eq(name)).
		One(ctx, s.rex(ctx))
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, apperr.ErrNotFound
	}
	return &authzdomain.RoleRecord{
		ID: uuidStr(r.ID), Name: r.Name, Description: r.Description,
		IsSystem: r.IsSystem, AllowedRealmKinds: r.AllowedRealmKinds,
	}, nil
}

func (s *Repository) CreateRole(ctx context.Context, tenantID string, r authzdomain.RoleRecord, applicationID string) (string, error) {
	row, ok, err := authzrquery.CreateRole.One(ctx, s.rex(ctx),
		tenantID, r.Name, r.Description, database.OptStr(applicationID), r.IsSystem,
		orDefaultKinds(r.AllowedRealmKinds), database.EmptyIfNil(r.AssignableAt))
	if err != nil {
		return "", database.MapErr(err)
	}
	if !ok {
		// INSERT … RETURNING yields a row or an error; no row means the
		// statement never ran.
		return "", apperr.ErrNotFound
	}
	return row.ID, nil
}

func (s *Repository) UpdateRole(ctx context.Context, tenantID string, r authzdomain.RoleRecord) error {
	_, err := authzrquery.UpdateRole.Exec(ctx, s.rex(ctx),
		r.ID, tenantID, r.Description,
		orDefaultKinds(r.AllowedRealmKinds), database.EmptyIfNil(r.AssignableAt))
	return database.MapErr(err)
}

func (s *Repository) RoleParents(ctx context.Context, roleID string) ([]string, error) {
	rows, err := authzrquery.ListRoleParents.Query(ctx, s.rex(ctx), roleID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.ParentID)
	}
	return out, nil
}

func (s *Repository) SetRoleParents(ctx context.Context, roleID string, parents []string) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := authzrquery.DeleteRoleParents.Exec(ctx, s.rex(ctx), roleID); err != nil {
			return database.MapErr(err)
		}
		for _, p := range parents {
			if _, err := authzrquery.InsertRoleParent.Exec(ctx, s.rex(ctx), roleID, p); err != nil {
				return database.MapErr(err)
			}
		}
		return nil
	})
}

func (s *Repository) RolePatterns(ctx context.Context, roleID string) ([]string, error) {
	rows, err := authzrquery.ListRolePatterns.Query(ctx, s.rex(ctx), roleID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.Pattern)
	}
	return out, nil
}

func (s *Repository) SetRolePatterns(ctx context.Context, roleID string, patterns []string) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := authzrquery.DeleteRolePatterns.Exec(ctx, s.rex(ctx), roleID); err != nil {
			return database.MapErr(err)
		}
		for _, p := range patterns {
			if _, err := authzrquery.InsertRolePattern.Exec(ctx, s.rex(ctx), roleID, p); err != nil {
				return database.MapErr(err)
			}
		}
		return nil
	})
}

func (s *Repository) SetRolePermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := authzrquery.DeleteRolePermissions.Exec(ctx, s.rex(ctx), roleID); err != nil {
			return database.MapErr(err)
		}
		for _, p := range permissionIDs {
			if _, err := authzrquery.InsertRolePermission.Exec(ctx, s.rex(ctx), roleID, p); err != nil {
				return database.MapErr(err)
			}
		}
		return nil
	})
}

func (s *Repository) AddRolePermission(ctx context.Context, roleID, permissionID string) error {
	_, err := authzrquery.InsertRolePermission.Exec(ctx, s.rex(ctx), roleID, permissionID)
	return database.MapErr(err)
}

func (s *Repository) RecomputeRole(ctx context.Context, roleID string) error {
	_, _, err := authzrquery.RecomputeRoleEffective.One(ctx, s.rex(ctx), roleID)
	return database.MapErr(err)
}

func (s *Repository) RolesBelow(ctx context.Context, roleID string) ([]string, error) {
	rows, err := authzrquery.RolesBelow.Query(ctx, s.rex(ctx), roleID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.RoleID)
	}
	return out, nil
}

func (s *Repository) RolesUsingPatterns(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := authzrquery.ListRolesUsingPattern.Query(ctx, s.rex(ctx), tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.RoleID)
	}
	return out, nil
}

func (s *Repository) RoleEffective(ctx context.Context, roleID string) ([]authzdomain.EffectivePermissionRecord, error) {
	rows, err := authzrquery.GetRoleEffective.Query(ctx, s.rex(ctx), roleID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authzdomain.EffectivePermissionRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, authzdomain.EffectivePermissionRecord{
			Key: r.PermissionKey.V, ViaRole: r.ViaRole,
		})
	}
	return out, nil
}

func orDefaultKinds(k []string) []string {
	if len(k) == 0 {
		return []string{"internal"}
	}
	return k
}
