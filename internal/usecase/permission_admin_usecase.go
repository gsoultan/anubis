package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/repository"
)

type PermissionAdminUsecase interface {
	ListPermissions(ctx context.Context, applicationSlug string, includeDeprecated bool) ([]repository.PermissionRecord, error)
}
