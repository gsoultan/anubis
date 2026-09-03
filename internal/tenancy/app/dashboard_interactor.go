package tenancyapp

import (
	"context"
	"fmt"
	"time"

	"github.com/gsoultan/anubis/internal/authz/guard"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// Dashboard is the overview's whole answer: measured facts only. Nothing
// here invents deltas or latency percentiles the server cannot know — the
// console renders what exists and omits what does not.
type Dashboard struct {
	IdentitiesByRealm []identitydomain.RealmCount
	GrantsTotal       int64
	ScopeNodesTotal   int64
	Decisions24h      int64
	Denies24h         int64
	Signals           []DashboardSignal
}

// DashboardSignal is one thing that deserves a human before any number.
type DashboardSignal struct {
	Kind     string // refresh_token_reuse | key_rotation_due | retention_overdue
	Severity string // page | alert
	Count    int64
	Detail   string
	Since    time.Time
}

// DashboardUsecase serves the console's overview.
type DashboardUsecase interface {
	GetDashboard(ctx context.Context) (*Dashboard, error)
}

// keyRotationPolicy matches the runbook: rotate every 30–90 days; past 90
// the overview says so out loud.
const keyRotationPolicy = 90 * 24 * time.Hour

type dashboardInteractor struct {
	guard      *guard.Guard
	identities tenancyport.IdentityStats
	grants     tenancyport.GrantStats
	nodes      tenancyport.ScopeStats
	decisions  tenancyport.DecisionStats
	ring       *keyring.Manager
	now        func() time.Time
}

func NewDashboardInteractor(
	ops guard.OperatorAuthority,
	clockNow func() time.Time,
	identities tenancyport.IdentityStats,
	grants tenancyport.GrantStats,
	nodes tenancyport.ScopeStats,
	decisions tenancyport.DecisionStats,
	ring *keyring.Manager,
) DashboardUsecase {
	return &dashboardInteractor{guard: guard.New().WithOperators(ops, clockNow),
		identities: identities, grants: grants,
		nodes: nodes, decisions: decisions, ring: ring, now: clockNow}
}

// GetDashboard gathers the overview. Gated on identity:read — every
// operator role holds it, and nothing here reveals more than the screens
// those roles can already open.
func (u *dashboardInteractor) GetDashboard(ctx context.Context) (*Dashboard, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	tenant := p.TenantID

	byRealm, err := u.identities.CountIdentitiesByRealm(ctx, tenant)
	if err != nil {
		return nil, err
	}
	grants, err := u.grants.CountLiveGrants(ctx, tenant)
	if err != nil {
		return nil, err
	}
	nodes, err := u.nodes.CountActiveScopeNodes(ctx, tenant)
	if err != nil {
		return nil, err
	}
	allows, denies, err := u.decisions.CountDecisions24h(ctx, tenant)
	if err != nil {
		return nil, err
	}

	d := &Dashboard{
		IdentitiesByRealm: byRealm,
		GrantsTotal:       grants,
		ScopeNodesTotal:   nodes,
		Decisions24h:      allows + denies,
		Denies24h:         denies,
	}

	// Signals, worst first. Each is a fact with a clock on it, never a
	// fabricated example.
	if n, latest, rerr := u.decisions.ReuseSignal(ctx, tenant); rerr == nil && n > 0 {
		d.Signals = append(d.Signals, DashboardSignal{
			Kind: "refresh_token_reuse", Severity: "page", Count: n,
			Detail: "A consumed refresh token was replayed. The family and session are already revoked; establish blast radius in the audit trail.",
			Since:  latest,
		})
	}
	if key, kerr := u.ring.Ring().ActiveAccess(); kerr == nil {
		if age := u.now().Sub(key.NotBefore); age > keyRotationPolicy {
			d.Signals = append(d.Signals, DashboardSignal{
				Kind: "key_rotation_due", Severity: "alert", Count: 1,
				Detail: fmt.Sprintf("Signing key kid=%s is %d days old (policy: 90).",
					key.Kid, int(age.Hours()/24)),
				Since: key.NotBefore.Add(keyRotationPolicy),
			})
		}
	}
	if n, berr := u.identities.CountRetentionBacklog(ctx, tenant); berr == nil && n > 0 {
		d.Signals = append(d.Signals, DashboardSignal{
			Kind: "retention_overdue", Severity: "alert", Count: n,
			Detail: fmt.Sprintf("%d records are past retention_until and not yet anonymised.", n),
			Since:  u.now(),
		})
	}
	return d, nil
}
