package service

import "github.com/gsoultan/anubis/internal/usecase"

type identityAdminService struct {
	usecase.IdentityAdminUsecase
}

func NewIdentityAdminService(uc usecase.IdentityAdminUsecase) IdentityAdminService {
	return &identityAdminService{IdentityAdminUsecase: uc}
}
