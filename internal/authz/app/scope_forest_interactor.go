package authzapp

import (
	"context"

	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// ScopeForestUsecase reads the live scope forest for the caller's tenant.
type ScopeForestUsecase interface {
	Execute(ctx context.Context, axes []string) ([]authzdomain.ForestNode, error)
}

type scopeForestInteractor struct {
	authz authzport.AuthzRepository
}

// NewScopeForestInteractor builds the use case.
func NewScopeForestInteractor(authz authzport.AuthzRepository) ScopeForestUsecase {
	return &scopeForestInteractor{authz: authz}
}

// Execute returns the forest, within the CALLER'S tenant.
//
// An empty axis list is refused rather than meaning "all": a PEP that asked for
// nothing and received an installation's entire scope graph would cache it by
// accident, and the mistake would look like working code.
func (u *scopeForestInteractor) Execute(ctx context.Context, axes []string) ([]authzdomain.ForestNode, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, apperr.ErrUnauthenticated
	}
	if len(axes) == 0 {
		return nil, apperr.ErrInvalidArgument.With("axes", "at least one axis is required")
	}
	for _, a := range axes {
		if a == "" {
			return nil, apperr.ErrInvalidArgument.With("axes", "an axis name cannot be empty")
		}
	}

	nodes, err := u.authz.ScopeForestForTenant(ctx, p.TenantID, axes)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	return nodes, nil
}
