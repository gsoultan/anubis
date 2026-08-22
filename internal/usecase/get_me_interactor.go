package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

// getMeInteractor implements GetMeUsecase.
type getMeInteractor struct {
	ids   repository.IdentityRepository
	authz repository.AuthzRepository
}

func NewGetMeInteractor(ids repository.IdentityRepository, authz repository.AuthzRepository) GetMeUsecase {
	return &getMeInteractor{ids: ids, authz: authz}
}

func (u *getMeInteractor) Execute(ctx context.Context) (*Me, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, domain.ErrUnauthenticated
	}
	identity, err := u.ids.Identity(ctx, p.TenantID, p.IdentityID)
	if err != nil || identity == nil {
		return nil, domain.ErrNotFound
	}
	roles, err := u.authz.RolesForIdentity(ctx, p.TenantID, p.IdentityID)
	if err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}
	perms, err := u.authz.EffectivePermissionsForIdentity(ctx, p.TenantID, p.IdentityID)
	if err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}
	return &Me{
		IdentityID:   identity.ID,
		Tenant:       p.TenantSlug,
		Realm:        identity.RealmCode,
		Username:     identity.Username,
		Email:        identity.Email,
		Roles:        roles,
		Permissions:  perms,
		ActiveScopes: p.Scopes,
		AMR:          p.AMR,
		IAL:          identity.AssuranceLevel,
		SessionID:    p.SessionID,
	}, nil
}
