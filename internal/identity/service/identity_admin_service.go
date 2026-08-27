package identitysvc

import identityapp "github.com/gsoultan/anubis/internal/identity/app"

// IdentityAdminService is the identity-administration surface.
type IdentityAdminService interface {
	identityapp.IdentityAdminUsecase
	identityapp.IdentityAttributesUsecase
}
