package authzapp

import (
	"context"

	authapp "github.com/gsoultan/anubis/internal/auth/app"
)

// SwitchScopeUsecase re-issues the caller's access token pinned to a
// different active scope, without re-authentication (mirrors AWS
// AssumeRole). The refresh chain is untouched.
type SwitchScopeUsecase interface {
	Execute(ctx context.Context, scopes map[string]string) (*authapp.TokenPair, error)
}
