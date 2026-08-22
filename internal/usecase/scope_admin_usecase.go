package usecase

// ScopeAdminUsecase composes scope administration (proto ScopeAdminService).
type ScopeAdminUsecase interface {
	ScopeAxisAdminUsecase
	ScopeNodeAdminUsecase
	ScopeSyncAdminUsecase
}
