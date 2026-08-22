package service

import "github.com/gsoultan/anubis/internal/usecase"

// TenantAdminService is the tenant-level administration surface.
type TenantAdminService interface {
	usecase.TenantAdminUsecase
}
