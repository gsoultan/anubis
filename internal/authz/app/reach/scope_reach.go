package authzreach

import "context"

// ScopeReach answers whether a subject holds ANY live grant that reaches a
// place on an axis. SwitchScope asks it before pinning a session there:
// a node that merely exists is not a node the caller is entitled to, and the
// SDK promises callers the difference. Implemented over the gate snapshot.
type ScopeReach interface {
	CanReach(ctx context.Context, tenantSlug, identityID, axis, nodeID string) (bool, error)
}
