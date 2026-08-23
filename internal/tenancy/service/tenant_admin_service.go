package tenancysvc

import tenancyapp "github.com/gsoultan/anubis/internal/tenancy/app"

// TenantAdminService is the tenant-level administration surface, including
// the sign-in and sign-out page builder.
type TenantAdminService interface {
	tenancyapp.TenantAdminUsecase
	tenancyapp.PageAdminUsecase
}
