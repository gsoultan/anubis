package controlsvc

import controlapp "github.com/gsoultan/anubis/internal/control/app"

type controlService struct {
	controlapp.OperatorAdminUsecase
	controlapp.PlatformAPIKeyUsecase
}

func NewControlService(uc controlapp.OperatorAdminUsecase, keys controlapp.PlatformAPIKeyUsecase) ControlService {
	return &controlService{OperatorAdminUsecase: uc, PlatformAPIKeyUsecase: keys}
}
