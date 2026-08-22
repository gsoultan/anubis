package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) ListRoles(ctx context.Context, tenantID, query string) ([]repository.RoleRecord, error) {
	rows, err := s.q(ctx).ListRoles(ctx, gen.ListRolesParams{TenantID: tenantID, Query: optStr(query)})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.RoleRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.RoleRecord{
			ID: r.ID, Name: r.Name, Description: r.Description,
			ApplicationSlug: deref(r.ApplicationSlug), IsSystem: r.IsSystem,
			AllowedRealmKinds: r.AllowedRealmKinds, AssignableAt: r.AssignableAt,
		})
	}
	return out, nil
}

func (s *Store) RoleByID(ctx context.Context, tenantID, id string) (*repository.RoleRecord, error) {
	r, err := s.q(ctx).GetRole(ctx, gen.GetRoleParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.RoleRecord{
		ID: r.ID, Name: r.Name, Description: r.Description,
		ApplicationSlug: deref(r.ApplicationSlug), IsSystem: r.IsSystem,
		AllowedRealmKinds: r.AllowedRealmKinds, AssignableAt: r.AssignableAt,
	}, nil
}

func (s *Store) RoleByName(ctx context.Context, tenantID, name string) (*repository.RoleRecord, error) {
	r, err := s.q(ctx).GetRoleByName(ctx, gen.GetRoleByNameParams{TenantID: tenantID, Name: name})
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.RoleRecord{
		ID: r.ID, Name: r.Name, Description: r.Description,
		IsSystem: r.IsSystem, AllowedRealmKinds: r.AllowedRealmKinds,
	}, nil
}

func (s *Store) CreateRole(ctx context.Context, tenantID string, r repository.RoleRecord, applicationID string) (string, error) {
	id, err := s.q(ctx).CreateRole(ctx, gen.CreateRoleParams{
		TenantID: tenantID, Name: r.Name, Description: r.Description,
		ApplicationID: optStr(applicationID), IsSystem: r.IsSystem,
		AllowedRealmKinds: orDefaultKinds(r.AllowedRealmKinds),
		AssignableAt:      emptyIfNil(r.AssignableAt),
	})
	return id, mapErr(err)
}

func (s *Store) UpdateRole(ctx context.Context, tenantID string, r repository.RoleRecord) error {
	_, err := s.q(ctx).UpdateRole(ctx, gen.UpdateRoleParams{
		ID: r.ID, TenantID: tenantID, Description: r.Description,
		AllowedRealmKinds: orDefaultKinds(r.AllowedRealmKinds),
		AssignableAt:      emptyIfNil(r.AssignableAt),
	})
	return mapErr(err)
}

func (s *Store) RoleParents(ctx context.Context, roleID string) ([]string, error) {
	out, err := s.q(ctx).ListRoleParents(ctx, roleID)
	return out, mapErr(err)
}

func (s *Store) SetRoleParents(ctx context.Context, roleID string, parents []string) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.q(ctx).DeleteRoleParents(ctx, roleID); err != nil {
			return mapErr(err)
		}
		for _, p := range parents {
			if err := s.q(ctx).InsertRoleParent(ctx, gen.InsertRoleParentParams{RoleID: roleID, ParentID: p}); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
}

func (s *Store) RolePatterns(ctx context.Context, roleID string) ([]string, error) {
	out, err := s.q(ctx).ListRolePatterns(ctx, roleID)
	return out, mapErr(err)
}

func (s *Store) SetRolePatterns(ctx context.Context, roleID string, patterns []string) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.q(ctx).DeleteRolePatterns(ctx, roleID); err != nil {
			return mapErr(err)
		}
		for _, p := range patterns {
			if err := s.q(ctx).InsertRolePattern(ctx, gen.InsertRolePatternParams{RoleID: roleID, Pattern: p}); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
}

func (s *Store) SetRolePermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.q(ctx).DeleteRolePermissions(ctx, roleID); err != nil {
			return mapErr(err)
		}
		for _, p := range permissionIDs {
			if err := s.q(ctx).InsertRolePermission(ctx, gen.InsertRolePermissionParams{RoleID: roleID, PermissionID: p}); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
}

func (s *Store) AddRolePermission(ctx context.Context, roleID, permissionID string) error {
	return mapErr(s.q(ctx).InsertRolePermission(ctx, gen.InsertRolePermissionParams{
		RoleID: roleID, PermissionID: permissionID,
	}))
}

func (s *Store) RecomputeRole(ctx context.Context, roleID string) error {
	return mapErr(s.q(ctx).RecomputeRoleEffective(ctx, roleID))
}

func (s *Store) RolesBelow(ctx context.Context, roleID string) ([]string, error) {
	out, err := s.q(ctx).RolesBelow(ctx, roleID)
	return out, mapErr(err)
}

func (s *Store) RolesUsingPatterns(ctx context.Context, tenantID string) ([]string, error) {
	out, err := s.q(ctx).ListRolesUsingPattern(ctx, tenantID)
	return out, mapErr(err)
}

func (s *Store) RoleEffective(ctx context.Context, roleID string) ([]repository.EffectivePermissionRecord, error) {
	rows, err := s.q(ctx).GetRoleEffective(ctx, roleID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.EffectivePermissionRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.EffectivePermissionRecord{
			Key: deref(r.PermissionKey), ViaRole: r.ViaRole,
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

func emptyIfNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
