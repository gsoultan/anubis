package service

import "github.com/gsoultan/anubis/internal/usecase"

// AuthzAdminService is the authorization-administration surface.
type AuthzAdminService interface {
	usecase.AuthzAdminUsecase
}
