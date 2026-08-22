package authzsvc

import authzadmin "github.com/gsoultan/anubis/internal/authz/app/admin"

type authzAdminService struct {
	authzadmin.AuthzAdminUsecase
}

func NewAuthzAdminService(uc authzadmin.AuthzAdminUsecase) AuthzAdminService {
	return &authzAdminService{AuthzAdminUsecase: uc}
}
