package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/repository"
)

type GrantAdminUsecase interface {
	ListGrants(ctx context.Context, identityID string, includeRevoked bool) ([]repository.GrantRecord, []repository.GrantScopeRecord, error)
	CreateGrant(ctx context.Context, in repository.GrantCreate) (string, error)
	RevokeGrant(ctx context.Context, grantID, reason string) error
}
