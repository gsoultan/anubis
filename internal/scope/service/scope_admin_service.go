package scopesvc

import scopeapp "github.com/gsoultan/anubis/internal/scope/app"

// ScopeAdminService is the scope-administration surface.
type ScopeAdminService interface {
	scopeapp.ScopeAdminUsecase
}
