package controlsvc

import controlapp "github.com/gsoultan/anubis/internal/control/app"

// ControlService is the platform administration surface.
type ControlService interface {
	controlapp.OperatorAdminUsecase
	controlapp.PlatformAPIKeyUsecase
}
