package sessionapp

import (
	"context"

	authzport "github.com/gsoultan/anubis/internal/authz/port"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// getMeInteractor implements GetMeUsecase.
type getMeInteractor struct {
	ids   identityport.IdentityRepository
	authz authzport.AuthzRepository
}

func NewGetMeInteractor(ids identityport.IdentityRepository, authz authzport.AuthzRepository) GetMeUsecase {
	return &getMeInteractor{ids: ids, authz: authz}
}

func (u *getMeInteractor) Execute(ctx context.Context) (*Me, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, apperr.ErrUnauthenticated
	}
	identity, err := u.ids.Identity(ctx, p.TenantID, p.IdentityID)
	if err != nil || identity == nil {
		return nil, apperr.ErrNotFound
	}
	roles, err := u.authz.RolesForIdentity(ctx, p.TenantID, p.IdentityID)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	perms, err := u.authz.EffectivePermissionsForIdentity(ctx, p.TenantID, p.IdentityID)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
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
