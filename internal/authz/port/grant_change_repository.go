package authzport

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/grantchange"
)

// GrantChangeRepository stores bulk changes to grants. Reads that lock
// (ChangeCandidates, GrantChange, CurrentStates) are meant to run inside the
// transaction that writes, so the plan is built on rows nobody else moves.
type GrantChangeRepository interface {
	// ChangeCandidates lists up to limit live grants of one role, with picks.
	ChangeCandidates(ctx context.Context, tenantID, roleID string, limit int) ([]grantchange.Candidate, error)
	AxisFacts(ctx context.Context, tenantID, axis string) (deny bool, rootID string, err error)
	// NodeFamily reads a place's kind, its siblings and the places inside it.
	NodeFamily(ctx context.Context, tenantID, nodeID string) (axis string, siblings, below []string, err error)
	RecordGrantChange(ctx context.Context, tenantID, actorID string, spec grantchange.Spec, items []grantchange.Item) (string, error)
	GrantChange(ctx context.Context, tenantID, id string) (*grantchange.Record, []grantchange.Item, error)
	CurrentStates(ctx context.Context, tenantID string, grantIDs []string) (map[string]grantchange.State, error)
	// RestoreGrantChange puts every grant back to its Before and marks the
	// change undone.
	RestoreGrantChange(ctx context.Context, tenantID, id, actorID string, items []grantchange.Item) error
	ListGrantChanges(ctx context.Context, tenantID string, limit int) ([]grantchange.Record, error)
}
