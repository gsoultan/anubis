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
	LoadedAt   time.Time

	// StrictAxes: axes with default_effect='deny' (active only).
	StrictAxes map[string]bool
	// Up[descendant][ancestor] = depth. Ancestor-or-self probe is two map hits.
	Up map[string]map[string]int16
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
