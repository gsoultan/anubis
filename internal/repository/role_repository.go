package repository

import "context"

type RoleRepository interface {
	ListRoles(ctx context.Context, tenantID, query string) ([]RoleRecord, error)
	RoleByID(ctx context.Context, tenantID, id string) (*RoleRecord, error)
	RoleByName(ctx context.Context, tenantID, name string) (*RoleRecord, error)
	CreateRole(ctx context.Context, tenantID string, r RoleRecord, applicationID string) (string, error)
	UpdateRole(ctx context.Context, tenantID string, r RoleRecord) error
	RoleParents(ctx context.Context, roleID string) ([]string, error)
	SetRoleParents(ctx context.Context, roleID string, parents []string) error
	RolePatterns(ctx context.Context, roleID string) ([]string, error)
	SetRolePatterns(ctx context.Context, roleID string, patterns []string) error
	SetRolePermissions(ctx context.Context, roleID string, permissionIDs []string) error
	AddRolePermission(ctx context.Context, roleID, permissionID string) error
	RecomputeRole(ctx context.Context, roleID string) error
	RolesBelow(ctx context.Context, roleID string) ([]string, error)
	RolesUsingPatterns(ctx context.Context, tenantID string) ([]string, error)
	RoleEffective(ctx context.Context, roleID string) ([]EffectivePermissionRecord, error)
}
