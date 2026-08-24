package provisioningsvc

import provisioningapp "github.com/gsoultan/anubis/internal/provisioning/app"

// ProvisioningService is the bulk provisioning surface.
type ProvisioningService interface {
	provisioningapp.ImportUsecase
}
