package service

import (
	"context"

	"github.com/gsoultan/anubis/internal/usecase"
)

type authzService struct {
	authorize   usecase.AuthorizeUsecase
	explain     usecase.ExplainUsecase
	switchScope usecase.SwitchScopeUsecase
}

func NewAuthzService(authorize usecase.AuthorizeUsecase, explain usecase.ExplainUsecase, switchScope usecase.SwitchScopeUsecase) AuthzService {
	return &authzService{authorize: authorize, explain: explain, switchScope: switchScope}
}

func (s *authzService) Authorize(ctx context.Context, in usecase.AuthorizeInput) (*usecase.Decision, error) {
	return s.authorize.Execute(ctx, in)
}

func (s *authzService) Explain(ctx context.Context, in usecase.AuthorizeInput) (*usecase.Explanation, error) {
	return s.explain.Execute(ctx, in)
}

func (s *authzService) SwitchScope(ctx context.Context, scopes map[string]string) (*usecase.TokenPair, error) {
	return s.switchScope.Execute(ctx, scopes)
}
