package service

import "github.com/gsoultan/anubis/internal/usecase"

type authzAdminService struct {
	usecase.AuthzAdminUsecase
}

func NewAuthzAdminService(uc usecase.AuthzAdminUsecase) AuthzAdminService {
	return &authzAdminService{AuthzAdminUsecase: uc}
}
