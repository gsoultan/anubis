package tenancyport

import (
	"context"
	"time"

	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
)

// The dashboard reads a little from several contexts. Each source is a
// narrow interface satisfied STRUCTURALLY by that context's repository —
// the provisioning pattern — so tenancy depends on capabilities, never on
// another context's adapter.

// IdentityStats is the identity context's contribution.
type IdentityStats interface {
	CountIdentitiesByRealm(ctx context.Context, tenantID string) ([]identitydomain.RealmCount, error)
	// CountRetentionBacklog is rows past retention_until and not yet
	// anonymised — each one a compliance clock already ringing.
	CountRetentionBacklog(ctx context.Context, tenantID string) (int64, error)
}

// GrantStats is the authz context's contribution.
type GrantStats interface {
	CountLiveGrants(ctx context.Context, tenantID string) (int64, error)
}

// ScopeStats is the scope context's contribution.
type ScopeStats interface {
	CountActiveScopeNodes(ctx context.Context, tenantID string) (int64, error)
}

// DecisionStats is the audit context's contribution. Authorize events are
// sampled under pressure, so these are floors, not exact counts.
type DecisionStats interface {
	CountDecisions24h(ctx context.Context, tenantID string) (allows, denies int64, err error)
	// ReuseSignal reports stolen-token events in the last week and when the
	// latest happened; zero means the banner stays down.
	ReuseSignal(ctx context.Context, tenantID string) (count int64, latest time.Time, err error)
}
