package guard

import (
	"context"
	"time"

	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// Guard gates the administration plane.
//
// Administration is performed by PLATFORM USERS and by nobody else
// (ADR-0011, as revised). Their authority is an assignment read on every
// call; a tenant identity — however many roles it holds — is refused
// outright, because a tenant's roles and permissions exist for the tenant's
// own people and applications, never for administering the tenant. The
// earlier "delegated administration" model, where a tenant person could be
// granted anubis:* permissions through a tenant role, was removed in
// migration 0029 along with the rows that made it possible.
type Guard struct {
	// ops resolves a platform user's assignments. Nil in wiring that never
	// sees one, in which case every caller is refused — the safe default.
	ops OperatorAuthority
	now func() time.Time
}

// OperatorAuthority is the control plane's answer to "what may this operator
// do, and where". Declared here rather than imported from the control
// context's port so the dependency points one way.
type OperatorAuthority interface {
	AssignmentsForOperator(ctx context.Context, operatorID string) ([]controldomain.AssignmentRecord, error)
}

// New returns a guard that refuses everyone: authority arrives only through
// WithOperators. A guard someone forgot to wire fails closed, not open.
func New() *Guard {
	return &Guard{now: time.Now}
}

// WithOperators teaches a guard where operator authority lives.
func (g *Guard) WithOperators(ops OperatorAuthority, now func() time.Time) *Guard {
	return &Guard{ops: ops, now: now}
}

// Require answers who is calling after proving they may administer this
// tenant. Only a platform principal can ever pass.
func (g *Guard) Require(ctx context.Context, permission string) (*authctx.Principal, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, apperr.ErrUnauthenticated
	}
	if !p.Platform {
		// Not a policy lookup that happens to deny — a different population.
		// The permission strings this plane checks exist only in the operator
		// role allow-lists; no tenant grant can confer them.
		return nil, apperr.ErrPermissionDenied.
			With("permission", permission).
			With("hint", "administration is performed by platform users")
	}
	return g.requirePlatform(ctx, p, permission)
}

// requirePlatform authorises an operator. The assignment is read on EVERY
// call rather than trusted from a token: revoking somebody's access has to
// take effect now, not whenever their token happens to expire.
func (g *Guard) requirePlatform(ctx context.Context, p *authctx.Principal, permission string) (*authctx.Principal, error) {
	if g.ops == nil {
		return nil, apperr.ErrPermissionDenied.With("permission", permission)
	}
	mine, err := g.ops.AssignmentsForOperator(ctx, p.IdentityID)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	now := g.now()

	// A global assignment is authority over the INSTALLATION, not over some
	// particular tenant, so installation-plane calls do not need one
	// selected. That is not a convenience: an owner creating the first
	// tenant has none to select, and requiring one would make a fresh
	// installation impossible to populate. Tenant-scoped calls still need a
	// tenant even under global authority — "any tenant" is not "no tenant",
	// and letting one through empty surfaces as an internal error deep in a
	// repository instead of a refusal the console can act on.
	for _, a := range mine {
		if a.Global() && a.Live(now) && a.Role.Allows(permission) {
			if p.TenantID == "" && !controldomain.InstallationScoped(permission) {
				return nil, apperr.ErrNoTenantSelected.
					With("permission", permission).
					With("hint", "send X-Anubis-Tenant")
			}
			return p, nil
		}
	}

	// Everything else is scoped to the tenant being administered.
	if p.TenantID == "" {
		return nil, apperr.ErrPermissionDenied.With("permission", permission)
	}
	for _, a := range mine {
		if a.Covers(p.TenantID, now) && a.Role.Allows(permission) {
			return p, nil
		}
	}
	return nil, apperr.ErrPermissionDenied.With("permission", permission)
}
