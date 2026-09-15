package scopeapp

// ScopeAdmin is everything the interactor is: the operator-facing surface and
// the scheduler's entry point, built once.
//
// Two callers take different halves of it. The RPC surface is given
// scopesvc.ScopeAdminService, which embeds only ScopeAdminUsecase — so the
// handler's own type has no RunDue on it. The maintenance scheduler is given
// ScopeSyncSchedulerUsecase, which has nothing else.
type ScopeAdmin interface {
	ScopeAdminUsecase
	ScopeSyncSchedulerUsecase
}
