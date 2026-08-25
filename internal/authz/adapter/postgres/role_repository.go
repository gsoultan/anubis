package authzpg

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/authz/adapter/postgres/gen"
	rrole "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rgen/role"
	authzrquery "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rquery"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) ListRoles(ctx context.Context, tenantID, query string) ([]authzdomain.RoleRecord, error) {
	rows, err := s.q(ctx).ListRoles(ctx, gen.ListRolesParams{TenantID: tenantID, Query: database.OptStr(query)})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authzdomain.RoleRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, authzdomain.RoleRecord{
			ID: r.ID, Name: r.Name, Description: r.Description,
			ApplicationSlug: database.Deref(r.ApplicationSlug), IsSystem: r.IsSystem,
			AllowedRealmKinds: r.AllowedRealmKinds, AssignableAt: r.AssignableAt,
		})
	}
	return out, nil
}

func (s *Repository) RoleByID(ctx context.Context, tenantID, id string) (*authzdomain.RoleRecord, error) {
	r, err := s.q(ctx).GetRole(ctx, gen.GetRoleParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &authzdomain.RoleRecord{
		ID: r.ID, Name: r.Name, Description: r.Description,
		ApplicationSlug: database.Deref(r.ApplicationSlug), IsSystem: r.IsSystem,
		AllowedRealmKinds: r.AllowedRealmKinds, AssignableAt: r.AssignableAt,
	}, nil
}

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
	_, err := s.q(ctx).UpdateRole(ctx, gen.UpdateRoleParams{
		ID: r.ID, TenantID: tenantID, Description: r.Description,
		AllowedRealmKinds: orDefaultKinds(r.AllowedRealmKinds),
		AssignableAt:      database.EmptyIfNil(r.AssignableAt),
	})
	return database.MapErr(err)
}

func (s *Repository) RoleParents(ctx context.Context, roleID string) ([]string, error) {
	out, err := s.q(ctx).ListRoleParents(ctx, roleID)
	return out, database.MapErr(err)
}

func (s *Repository) SetRoleParents(ctx context.Context, roleID string, parents []string) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.q(ctx).DeleteRoleParents(ctx, roleID); err != nil {
			return database.MapErr(err)
		}
		for _, p := range parents {
			if err := s.q(ctx).InsertRoleParent(ctx, gen.InsertRoleParentParams{RoleID: roleID, ParentID: p}); err != nil {
				return database.MapErr(err)
			}
		}
		return nil
	})
}

func (s *Repository) RolePatterns(ctx context.Context, roleID string) ([]string, error) {
	out, err := s.q(ctx).ListRolePatterns(ctx, roleID)
	return out, database.MapErr(err)
}

func (s *Repository) SetRolePatterns(ctx context.Context, roleID string, patterns []string) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.q(ctx).DeleteRolePatterns(ctx, roleID); err != nil {
			return database.MapErr(err)
		}
		for _, p := range patterns {
			if err := s.q(ctx).InsertRolePattern(ctx, gen.InsertRolePatternParams{RoleID: roleID, Pattern: p}); err != nil {
				return database.MapErr(err)
			}
		}
		return nil
	})
}

func (s *Repository) SetRolePermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.q(ctx).DeleteRolePermissions(ctx, roleID); err != nil {
			return database.MapErr(err)
		}
		for _, p := range permissionIDs {
			if err := s.q(ctx).InsertRolePermission(ctx, gen.InsertRolePermissionParams{RoleID: roleID, PermissionID: p}); err != nil {
				return database.MapErr(err)
			}
		}
		return nil
	})
}

func (s *Repository) AddRolePermission(ctx context.Context, roleID, permissionID string) error {
	return database.MapErr(s.q(ctx).InsertRolePermission(ctx, gen.InsertRolePermissionParams{
		RoleID: roleID, PermissionID: permissionID,
	}))
}

func (s *Repository) RecomputeRole(ctx context.Context, roleID string) error {
	return database.MapErr(s.q(ctx).RecomputeRoleEffective(ctx, roleID))
}

func (s *Repository) RolesBelow(ctx context.Context, roleID string) ([]string, error) {
	out, err := s.q(ctx).RolesBelow(ctx, roleID)
	return out, database.MapErr(err)
}

func (s *Repository) RolesUsingPatterns(ctx context.Context, tenantID string) ([]string, error) {
	out, err := s.q(ctx).ListRolesUsingPattern(ctx, tenantID)
	return out, database.MapErr(err)
}

func (s *Repository) RoleEffective(ctx context.Context, roleID string) ([]authzdomain.EffectivePermissionRecord, error) {
	rows, err := s.q(ctx).GetRoleEffective(ctx, roleID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authzdomain.EffectivePermissionRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, authzdomain.EffectivePermissionRecord{
			Key: database.Deref(r.PermissionKey), ViaRole: r.ViaRole,
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
