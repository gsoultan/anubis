package service

import (
	"context"

	"github.com/gsoultan/anubis/internal/usecase"
)

type tokenService struct {
	introspect usecase.IntrospectUsecase
	revoke     usecase.RevokeUsecase
}

func NewTokenService(introspect usecase.IntrospectUsecase, revoke usecase.RevokeUsecase) TokenService {
	return &tokenService{introspect: introspect, revoke: revoke}
}

func (s *tokenService) Introspect(ctx context.Context, token string) (*usecase.IntrospectResult, error) {
	return s.introspect.Execute(ctx, token)
}

func (s *tokenService) Revoke(ctx context.Context, token, hint string) error {
	return s.revoke.Execute(ctx, token, hint)
}
