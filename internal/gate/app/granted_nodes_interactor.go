package gateapp

import (
	"context"

	authzreach "github.com/gsoultan/anubis/internal/authz/app/reach"
	"github.com/gsoultan/anubis/internal/authz/domain/granted"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/clock"
	"github.com/gsoultan/anubis/internal/shared/validate"
)

// GrantedNodesInteractor answers authz's "where can they" and "can they
// reach" questions from the gate's snapshot. It is the gate context's
// implementation of two authz usecases (GrantedNodesUsecase, ScopeReach),
// satisfied structurally, so authz never imports the snapshot.
//
// The snapshot is the right source and the only honest one: it is what the
// gate decides with, so a list drawn from it agrees with every decision the
// gate makes. Stale is refused, not served — a list built from an old
// snapshot would hand an application places a revoked grant used to reach.
type GrantedNodesInteractor struct {
	snaps Snapshots
	clock clock.Clock
}

func NewGrantedNodesInteractor(snaps Snapshots, clk clock.Clock) *GrantedNodesInteractor {
	return &GrantedNodesInteractor{snaps: snaps, clock: clk}
}

// Execute implements authzreach.NodesUsecase.
func (u *GrantedNodesInteractor) Execute(ctx context.Context, in authzreach.NodesInput) (*granted.Nodes, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, apperr.ErrUnauthenticated
	}
	if in.Subject == "" || in.Permission == "" || !validate.ValidCode(in.Axis) {
		return nil, apperr.ErrInvalidArgument.With("hint", "subject, permission and axis are required")
	}
	if len(in.Scopes) > 16 {
		return nil, apperr.ErrInvalidArgument.With("scopes", "too many axes")
	}
	for axis := range in.Scopes {
		if !validate.ValidCode(axis) {
			return nil, apperr.ErrInvalidArgument.With("axis", axis)
		}
	}
	snap, fresh := u.snaps.Get(p.TenantSlug)
	if snap == nil || !fresh {
		return nil, apperr.ErrSnapshotUnavailable
	}
	out, known := snap.GrantedNodes(in.Subject, in.Permission, in.Axis, in.Scopes, u.clock.Now())
	if !known {
		// Not a denial: the application asked about a subject or a
		// permission this tenant does not have, which is a wiring mistake
		// it should see, not an empty list it would treat as "nowhere".
		return nil, apperr.ErrNotFound.With("hint", "unknown subject or permission")
	}
	return &out, nil
}

// CanReach implements authzreach.ScopeReach.
func (u *GrantedNodesInteractor) CanReach(_ context.Context, tenantSlug, identityID, axis, nodeID string) (bool, error) {
	snap, fresh := u.snaps.Get(tenantSlug)
	if snap == nil || !fresh {
		return false, apperr.ErrSnapshotUnavailable
	}
	return snap.Reaches(identityID, axis, nodeID, u.clock.Now()), nil
}
