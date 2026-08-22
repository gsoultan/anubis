package authzadmin

import (
	"context"

	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
)

type RoleAdminUsecase interface {
	ListRoles(ctx context.Context, query string) ([]authzdomain.RoleRecord, map[string][]string, map[string][]string, error)
	CreateRole(ctx context.Context, r authzdomain.RoleRecord, parents, patterns []string) (*authzdomain.RoleRecord, error)
	UpdateRole(ctx context.Context, r authzdomain.RoleRecord, parents, patterns []string) (*authzdomain.RoleRecord, error)
	GetRoleEffective(ctx context.Context, roleID string) ([]authzdomain.EffectivePermissionRecord, error)
}
