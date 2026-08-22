package authzadmin

import (
	"context"

	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
)

type PermissionAdminUsecase interface {
	ListPermissions(ctx context.Context, applicationSlug string, includeDeprecated bool) ([]authzdomain.PermissionRecord, error)
}
