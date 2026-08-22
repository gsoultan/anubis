package service

import "github.com/gsoultan/anubis/internal/usecase"

// IdentityAdminService is the identity-administration surface.
type IdentityAdminService interface {
	usecase.IdentityAdminUsecase
}
