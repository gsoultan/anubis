package identitysvc

import identityapp "github.com/gsoultan/anubis/internal/identity/app"

type identityAdminService struct {
	identityapp.IdentityAdminUsecase
}

func NewIdentityAdminService(uc identityapp.IdentityAdminUsecase) IdentityAdminService {
	return &identityAdminService{IdentityAdminUsecase: uc}
}
