package usecase

// IdentityAdminUsecase is the composed identity-administration surface
// (proto IdentityAdminService). Composition, not width.
type IdentityAdminUsecase interface {
	IdentityLifecycleUsecase
	CredentialAdminUsecase
	ConsentAdminUsecase
}
