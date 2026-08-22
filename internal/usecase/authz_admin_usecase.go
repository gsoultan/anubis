package usecase

// AuthzAdminUsecase composes the authorization-administration surface
// (proto AuthzAdminService).
type AuthzAdminUsecase interface {
	RoleAdminUsecase
	PermissionAdminUsecase
	GrantAdminUsecase
	MembershipAdminUsecase
	ManifestAdminUsecase
}
