package tenancysvc

import tenancyapp "github.com/gsoultan/anubis/internal/tenancy/app"

// TenantAdminService is the tenant-level administration surface.
type TenantAdminService interface {
	tenancyapp.TenantAdminUsecase
}
