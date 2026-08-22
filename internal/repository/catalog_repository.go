package repository

// CatalogRepository is the composite admin plane. Composition, not width:
// every embedded interface stays within the 15-method budget.
type CatalogRepository interface {
	RealmAdminRepository
	IdentityDirectoryRepository
	ApplicationRepository
	RouteRepository
	RoleRepository
	PermissionCatalogRepository
	GrantRepository
	MembershipRepository
	ScopeAxisRepository
	ScopeNodeRepository
	ScopeSyncRepository
	ConsentRepository
	AuditReadRepository
	SigninPageRepository
	BackchannelDirectoryRepository
}
