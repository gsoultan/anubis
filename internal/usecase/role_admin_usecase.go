package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/repository"
)

type RoleAdminUsecase interface {
	ListRoles(ctx context.Context, query string) ([]repository.RoleRecord, map[string][]string, map[string][]string, error)
	CreateRole(ctx context.Context, r repository.RoleRecord, parents, patterns []string) (*repository.RoleRecord, error)
	UpdateRole(ctx context.Context, r repository.RoleRecord, parents, patterns []string) (*repository.RoleRecord, error)
	GetRoleEffective(ctx context.Context, roleID string) ([]repository.EffectivePermissionRecord, error)
}
