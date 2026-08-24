package controlsvc

import controlapp "github.com/gsoultan/anubis/internal/control/app"

type controlService struct {
	controlapp.OperatorAdminUsecase
}

func NewControlService(uc controlapp.OperatorAdminUsecase) ControlService {
	return &controlService{OperatorAdminUsecase: uc}
}
