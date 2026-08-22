package tenancysvc

import tenancyapp "github.com/gsoultan/anubis/internal/tenancy/app"

type tenantAdminService struct {
	tenancyapp.TenantAdminUsecase
}

func NewTenantAdminService(uc tenancyapp.TenantAdminUsecase) TenantAdminService {
	return &tenantAdminService{TenantAdminUsecase: uc}
}
