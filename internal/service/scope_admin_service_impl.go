package service

import "github.com/gsoultan/anubis/internal/usecase"

type scopeAdminService struct {
	usecase.ScopeAdminUsecase
}

func NewScopeAdminService(uc usecase.ScopeAdminUsecase) ScopeAdminService {
	return &scopeAdminService{ScopeAdminUsecase: uc}
}
