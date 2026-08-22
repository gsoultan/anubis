package authzsvc

import (
	"context"

	authapp "github.com/gsoultan/anubis/internal/auth/app"
	authzapp "github.com/gsoultan/anubis/internal/authz/app"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
)

type authzService struct {
	authorize   authzapp.AuthorizeUsecase
	explain     authzapp.ExplainUsecase
	switchScope authzapp.SwitchScopeUsecase
}

func NewAuthzService(authorize authzapp.AuthorizeUsecase, explain authzapp.ExplainUsecase, switchScope authzapp.SwitchScopeUsecase) AuthzService {
	return &authzService{authorize: authorize, explain: explain, switchScope: switchScope}
}

func (s *authzService) Authorize(ctx context.Context, in authzapp.AuthorizeInput) (*authzdomain.Decision, error) {
	return s.authorize.Execute(ctx, in)
}

func (s *authzService) Explain(ctx context.Context, in authzapp.AuthorizeInput) (*authzdomain.Explanation, error) {
	return s.explain.Execute(ctx, in)
}

func (s *authzService) SwitchScope(ctx context.Context, scopes map[string]string) (*authapp.TokenPair, error) {
	return s.switchScope.Execute(ctx, scopes)
}
