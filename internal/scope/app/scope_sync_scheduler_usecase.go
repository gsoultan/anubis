package scopeapp

import (
	"context"
	"time"
)

// ScopeSyncSchedulerUsecase is the timer's half of scope sync, and it is
// deliberately not part of ScopeAdminUsecase.
//
// RunDue checks no permission and takes no tenant from the caller, because
// there is no caller: the authority for the run was settled when an operator
// configured the source. That is correct for a clock and indefensible on
// anything a request can reach, so the transport is handed ScopeAdminService —
// a type that does not have the method at all — rather than a rule saying not
// to call it. scripts/check/import-boundary.sh fails the build if an adapter
// names RunDue anyway.
type ScopeSyncSchedulerUsecase interface {
	RunDue(ctx context.Context, now time.Time, limit int32) (ran int, err error)
}
