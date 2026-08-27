package identitysvc

import identityapp "github.com/gsoultan/anubis/internal/identity/app"

type identityAdminService struct {
	identityapp.IdentityAdminUsecase
	// Composed rather than merged into the interactor: sealing needs the
	// master key, and threading that through everything that administers an
	// identity would put it in reach of code that has no business holding it.
	identityapp.IdentityAttributesUsecase
}

func NewIdentityAdminService(
	uc identityapp.IdentityAdminUsecase,
	attrs identityapp.IdentityAttributesUsecase,
) IdentityAdminService {
	return &identityAdminService{IdentityAdminUsecase: uc, IdentityAttributesUsecase: attrs}
}
