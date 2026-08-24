package controlapp

import (
	"context"

	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	controlport "github.com/gsoultan/anubis/internal/control/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
)

// platformGuard authorises platform users.
//
// It is deliberately NOT authz's guard. An operator holds no grants — they
// are not a member of any tenant — so authorize() would deny them
// everything. Their authority is platform_assignments, and reading it here
// also keeps the dependency pointing one way: control already depends on
// authz, and teaching authz's guard about platform users would close that
// loop.
type platformGuard struct {
	read  controlport.AssignmentReader
	clock clock.Clock
}

// require proves the caller is a platform user whose live assignments carry
// the permission somewhere, and returns those assignments.
//
// A tenant identity never satisfies this, however many roles it holds: the
// two populations are separate, and a token that names one is not a token
// that names the other.
func (g platformGuard) require(ctx context.Context, permission string) (*authctx.Principal, []controldomain.AssignmentRecord, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, nil, apperr.ErrUnauthenticated
	}
	if !p.Platform {
		return nil, nil, apperr.ErrPermissionDenied.With("permission", permission)
	}
	all, err := g.read.Assignments(ctx)
	if err != nil {
		return nil, nil, err
	}
	now := g.clock.Now()
	mine := make([]controldomain.AssignmentRecord, 0, 4)
	allowed := false
	for _, a := range all {
		if a.OperatorID != p.IdentityID || !a.Live(now) {
			continue
		}
		mine = append(mine, a)
		if a.Role.Allows(permission) {
			allowed = true
		}
	}
	if len(mine) == 0 {
		// An operator with no live assignment can sign in and reach nothing.
		return nil, nil, apperr.ErrPermissionDenied.With("permission", permission)
	}
	if !allowed {
		return nil, nil, apperr.ErrPermissionDenied.With("permission", permission)
	}
	return p, mine, nil
}

// requireAny proves the caller is an operator at all, for reads that any of
// them may perform.
func (g platformGuard) requireAny(ctx context.Context) (*authctx.Principal, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, apperr.ErrUnauthenticated
	}
	if !p.Platform {
		return nil, apperr.ErrPermissionDenied
	}
	all, err := g.read.Assignments(ctx)
	if err != nil {
		return nil, err
	}
	now := g.clock.Now()
	for _, a := range all {
		if a.OperatorID == p.IdentityID && a.Live(now) {
			return p, nil
		}
	}
	return nil, apperr.ErrPermissionDenied
}
