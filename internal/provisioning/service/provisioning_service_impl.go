package provisioningsvc

import provisioningapp "github.com/gsoultan/anubis/internal/provisioning/app"

type provisioningService struct {
	provisioningapp.ImportUsecase
}

func NewProvisioningService(uc provisioningapp.ImportUsecase) ProvisioningService {
	return &provisioningService{ImportUsecase: uc}
}
