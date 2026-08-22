package authzsvc

import authzadmin "github.com/gsoultan/anubis/internal/authz/app/admin"

// AuthzAdminService is the authorization-administration surface.
type AuthzAdminService interface {
	authzadmin.AuthzAdminUsecase
}
