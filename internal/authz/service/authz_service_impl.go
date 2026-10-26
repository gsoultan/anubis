package authzsvc

import (
	"context"

	authapp "github.com/gsoultan/anubis/internal/auth/app"
	authzapp "github.com/gsoultan/anubis/internal/authz/app"
	authzreach "github.com/gsoultan/anubis/internal/authz/app/reach"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/authz/domain/granted"
)

type authzService struct {
	authorize   authzapp.AuthorizeUsecase
	explain     authzapp.ExplainUsecase
	switchScope authzapp.SwitchScopeUsecase
	granted     authzreach.NodesUsecase
}

func NewAuthzService(authorize authzapp.AuthorizeUsecase, explain authzapp.ExplainUsecase, switchScope authzapp.SwitchScopeUsecase, granted authzreach.NodesUsecase) AuthzService {
	return &authzService{authorize: authorize, explain: explain, switchScope: switchScope, granted: granted}
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

func (s *authzService) ListGrantedNodes(ctx context.Context, in authzreach.NodesInput) (*granted.Nodes, error) {
	return s.granted.Execute(ctx, in)
}
