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
	grants      authzapp.EffectiveGrantsUsecase
	forest      authzapp.ScopeForestUsecase
}

func NewAuthzService(authorize authzapp.AuthorizeUsecase, explain authzapp.ExplainUsecase, switchScope authzapp.SwitchScopeUsecase, grants authzapp.EffectiveGrantsUsecase, forest authzapp.ScopeForestUsecase) AuthzService {
	return &authzService{authorize: authorize, explain: explain, switchScope: switchScope, grants: grants, forest: forest}
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

func (s *authzService) ListEffectiveGrants(ctx context.Context, subject string) ([]authzdomain.EffectiveGrant, error) {
	return s.grants.Execute(ctx, subject)
}

func (s *authzService) GetScopeForest(ctx context.Context, axes []string) ([]authzdomain.ForestNode, error) {
	return s.forest.Execute(ctx, axes)
}
