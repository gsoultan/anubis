package tenancysvc

import tenancyapp "github.com/gsoultan/anubis/internal/tenancy/app"

// tenantAdminService composes the two halves of tenant administration: the
// tenant/realm/application surface and the page builder. They are separate
// usecases because a delegated admin may hold one permission and not the
// other — branding a portal is not the same authority as rotating a client
// secret.
type tenantAdminService struct {
	tenancyapp.TenantAdminUsecase
	tenancyapp.PageAdminUsecase
	tenancyapp.DashboardUsecase
}

func NewTenantAdminService(uc tenancyapp.TenantAdminUsecase, pages tenancyapp.PageAdminUsecase, dash tenancyapp.DashboardUsecase) TenantAdminService {
	return &tenantAdminService{TenantAdminUsecase: uc, PageAdminUsecase: pages, DashboardUsecase: dash}
}
