package authzapp

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// EffectiveGrantsUsecase reads one identity's live grants.
type EffectiveGrantsUsecase interface {
	Execute(ctx context.Context, subject string) ([]authzdomain.EffectiveGrant, error)
}

type effectiveGrantsInteractor struct {
	authz authzport.AuthzRepository
}

// NewEffectiveGrantsInteractor builds the use case.
func NewEffectiveGrantsInteractor(authz authzport.AuthzRepository) EffectiveGrantsUsecase {
	return &effectiveGrantsInteractor{authz: authz}
}

// Execute returns the subject's live grants, within the CALLER'S tenant.
//
// The tenant comes from the authenticated principal and never from the request.
// A subject id is not a secret and is frequently known across tenant
// boundaries, so taking one from the caller would turn this into a read of any
// identity in the installation — which is the thing closing the admin plane was
// protecting, reopened through a convenience.
func (u *effectiveGrantsInteractor) Execute(ctx context.Context, subject string) ([]authzdomain.EffectiveGrant, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, apperr.ErrUnauthenticated
	}
	if subject == "" {
		return nil, apperr.ErrInvalidArgument
	}

	grants, err := u.authz.EffectiveGrantsForIdentity(ctx, p.TenantID, subject)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	// An identity with no grants, and one in another tenant, are both an empty
	// list. Deliberately indistinguishable: answering "no such identity" would
	// make this an existence oracle across tenants.
	return grants, nil
}
