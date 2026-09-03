// Package snapshot is the in-memory authorization state the gate serves
// from: no database on the request path, fail-static under outage,
// fail-closed past max age. Pure data + evaluation; loading lives in the
// postgres repository (the SQL boundary), refresh in Manager.
package snapshot

import "time"

// Data is one tenant's frozen catalog, loaded in ONE REPEATABLE READ
// transaction (ADR-0005 §10) — never assembled from separate reads.
type Data struct {
	TenantID   string
	TenantSlug string
	Version    int64
	// LoadedAt: when this snapshot was last confirmed current — which the
	// version gate can do without rebuilding. BuiltAt: when it was last
	// actually rebuilt from the database. They differ, and the difference is
	// the point: freshness is judged on LoadedAt, but the Manager forces a
	// rebuild once BuiltAt falls too far behind, so a lost invalidation
	// trigger cannot keep a snapshot alive forever.
	LoadedAt time.Time
	BuiltAt  time.Time

	// StrictAxes: axes with default_effect='deny' (active only).
	StrictAxes map[string]bool
	// Scope: the node hierarchy as parent pointers. Ancestor-or-self is one
	// map hit to resolve the target, then a walk. See ScopeIndex.
	Scope ScopeIndex
	// GrantsByIdentity holds live grants with their per-axis constraints.
	GrantsByIdentity map[string][]Grant
	// RolePermissions[roleID][permissionKey] = true (effective, flattened).
	RolePermissions map[string]map[string]bool
	// Permissions by key.
	Permissions map[string]Permission
	// Identities: the state gates that outrank grants.
	Identities map[string]Identity
	// RevokedSessions within the max access-token TTL window.
	RevokedSessions map[string]bool
	// RoutesByHost: route policies grouped per application slug, sorted by
	// priority ascending (explicit priority beats implicit specificity).
	Routes []Route
}

// InternGrantScopes resolves every grant constraint against the scope index.
// The loader MUST call this once, after both the index and the grants are
// populated. Until it runs every constraint is unresolved, so the gate denies
// rather than granting on a half-built snapshot.
func (d *Data) InternGrantScopes() {
	for _, grants := range d.GrantsByIdentity {
		for i := range grants {
			for axis := range grants[i].Scopes {
				d.Scope.Intern(grants[i].Scopes[axis])
			}
		}
	}
}

type Grant struct {
	ID         string
	RoleID     string
	SelfScoped bool
	ValidFrom  time.Time
	ValidUntil time.Time // zero = open-ended
	// Scopes per axis: OR within the axis, AND across axes.
	Scopes map[string][]ScopeConstraint
}

type ScopeConstraint struct {
	NodeID  string
	Inherit bool
	// node is the ScopeIndex index PLUS ONE. The offset is deliberate: the
	// zero value has to mean "not yet interned", because a plain 0 would be
	// a VALID index and an un-interned constraint would silently match
	// whichever node loaded first — fail-open, in the one place we cannot
	// afford it. Unresolved stays unresolved and denies.
	// Set by ScopeIndex.Intern via Data.InternGrantScopes.
	node int32
}

type Permission struct {
	Key            string
	MinAssurance   int
	RequiresAMR    []string
	MaxAuthAgeSecs int64
	Risk           string
}

type Identity struct {
	TokenEpoch     int
	Blocked        bool
	AssuranceLevel int
}

type Route struct {
	AppSlug       string
	Priority      int
	Effect        string // public | require_auth | require_permission | deny
	PathPattern   string
	HostPattern   string
	Methods       []string
	PermissionKey string
	// ScopeBindings: axis -> "token" | "path.<param>"
	ScopeBindings map[string]string
}
