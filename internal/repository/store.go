package repository

// Store is everything the postgres implementation provides. Constructors
// hand usecases the narrow interfaces; only wiring sees the whole.
type Store interface {
	TxManager
	IdentityRepository
	CredentialRepository
	RealmRepository
	SessionRepository
	RefreshRepository
	OneTimeRepository
	KeyRepository
	AuthzRepository
	TenantRepository
	AuditRepository
	CatalogRepository
}
