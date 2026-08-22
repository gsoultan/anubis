package authsvc

import (
	"context"

	tokenapp "github.com/gsoultan/anubis/internal/auth/app/token"
)

type tokenService struct {
	introspect tokenapp.IntrospectUsecase
	revoke     tokenapp.RevokeUsecase
}

func NewTokenService(introspect tokenapp.IntrospectUsecase, revoke tokenapp.RevokeUsecase) TokenService {
	return &tokenService{introspect: introspect, revoke: revoke}
}

func (s *tokenService) Introspect(ctx context.Context, token string) (*tokenapp.IntrospectResult, error) {
	return s.introspect.Execute(ctx, token)
}

func (s *tokenService) Revoke(ctx context.Context, token, hint string) error {
	return s.revoke.Execute(ctx, token, hint)
}
