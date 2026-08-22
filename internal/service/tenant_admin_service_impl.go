package service

import "github.com/gsoultan/anubis/internal/usecase"

type tenantAdminService struct {
	usecase.TenantAdminUsecase
}

func NewTenantAdminService(uc usecase.TenantAdminUsecase) TenantAdminService {
	return &tenantAdminService{TenantAdminUsecase: uc}
}
