package scopesvc

import scopeapp "github.com/gsoultan/anubis/internal/scope/app"

type scopeAdminService struct {
	scopeapp.ScopeAdminUsecase
}

func NewScopeAdminService(uc scopeapp.ScopeAdminUsecase) ScopeAdminService {
	return &scopeAdminService{ScopeAdminUsecase: uc}
}
