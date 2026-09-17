package gaterquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// AxisRow is one scope axis the gate evaluates against.
type AxisRow struct {
	Code          string
	DefaultEffect string
	Status        string
	SortOrder     int32
}

var SnapshotAxes = storm.SQL[AxisRow](`
SELECT code, default_effect, status, sort_order FROM scope_axes
WHERE status = 'active'`)

// NodeRow is one scope node as a PARENT POINTER.
type NodeRow struct {
	ID       string
	ParentID runtime.Null[string]
}

// SnapshotNodes reads the hierarchy as parent pointers, not as a materialised
// closure.
//
// The gate only ever asks "is granted node A an ancestor-or-self of target B",
// which a walk up parent_id answers exactly — and one row per node instead of
// one per (node, ancestor) pair. At 1M nodes that is 1M rows rather than 4M,
// and the in-memory form stops growing with tree depth.
//
// NO status FILTER, DELIBERATELY. authorize() (migration 0013) probes
// scope_closure without looking at scope_nodes.status, so an archived node
// still carries grants and still resolves its ancestors. Filtering to 'active'
// here would make the gate DENY what the SQL engine ALLOWS, and would break
// the chain under any archived intermediate node. snapshot_parity_test.go is
// what catches this.
var SnapshotNodes = storm.SQL[NodeRow](`
SELECT id::text AS id, parent_id::text AS parent_id FROM scope_nodes
WHERE tenant_id = $1`)

// GrantRow is one live grant.
type GrantRow struct {
	ID         string
	IdentityID string
	RoleID     string
	SelfScoped bool
	ValidFrom  time.Time
	ValidUntil runtime.Null[time.Time]
}

var SnapshotGrants = storm.SQL[GrantRow](`
SELECT g.id::text AS id, g.identity_id::text AS identity_id,
       g.role_id::text AS role_id, g.self_scoped, g.valid_from, g.valid_until
FROM grants g
WHERE g.tenant_id = $1 AND g.revoked_at IS NULL`)

// GrantScopeRow is one scope binding on a grant.
type GrantScopeRow struct {
	GrantID     string
	AxisCode    string
	ScopeNodeID string
	Inherit     bool
	Exclude     bool
}

// SnapshotGrantScopes carries mode as the BOOL the evaluator branches on.
//
// A gate that loaded the includes and dropped the excludes would allow, in
// memory and at p99 < 1 ms, exactly what the database denies — the worst
// direction for those two to disagree in. snapshot_parity_test.go probes
// carve-outs for this reason.
var SnapshotGrantScopes = storm.SQL[GrantScopeRow](`
SELECT gs.grant_id::text AS grant_id, gs.axis_code,
       gs.scope_node_id::text AS scope_node_id, gs.inherit,
       gs.mode = 'exclude' AS exclude
FROM grant_scopes gs
WHERE gs.tenant_id = $1`)

// RolePermissionRow is one edge of the effective role -> permission set.
type RolePermissionRow struct {
	RoleID string
	Key    string
}

var SnapshotRolePermissions = storm.SQL[RolePermissionRow](`
SELECT rpe.role_id::text AS role_id, p.key
FROM role_permissions_effective rpe
JOIN permissions p ON p.id = rpe.permission_id
WHERE p.tenant_id = $1 AND p.deprecated_at IS NULL`)

// PermissionRow is one permission and the assurance it demands.
type PermissionRow struct {
	Key            string
	Risk           string
	MinAssurance   int16
	RequiresAmr    []string
	MaxAuthAgeSecs int64
}

// SnapshotPermissions renders max_auth_age as SECONDS.
//
// COALESCE to 0 because an absent age is "no recency requirement", and the
// evaluator compares numbers: a NULL reaching it would have to be a third
// case everywhere it is read.
var SnapshotPermissions = storm.SQL[PermissionRow](`
SELECT p.key, p.risk, p.min_assurance, p.requires_amr,
       COALESCE(extract(epoch FROM p.max_auth_age), 0)::bigint AS max_auth_age_secs
FROM permissions p
WHERE p.tenant_id = $1 AND p.deprecated_at IS NULL`)

// IdentityRow is one identity's gate-relevant state.
type IdentityRow struct {
	ID             string
	TokenEpoch     int32
	Status         string
	AssuranceLevel int16
	Blocked        bool
}

// SnapshotIdentities folds disabled and anonymized into one BLOCKED flag: the
// gate's decision is the same for both, and two flags would be two places to
// forget one.
var SnapshotIdentities = storm.SQL[IdentityRow](`
SELECT id::text AS id, token_epoch, status, assurance_level,
       (disabled_at IS NOT NULL OR anonymized_at IS NOT NULL) AS blocked
FROM identities
WHERE tenant_id = $1`)

// RouteRow is one gate rule, with its application and permission resolved.
type RouteRow struct {
	ID              string
	Priority        int32
	Effect          string
	PathPattern     string
	HostPattern     runtime.Null[string]
	Methods         []string
	ScopeBindings   runtime.JSON
	PermissionKey   runtime.Null[string]
	ApplicationSlug string
}

// SnapshotRoutes is ordered by (application, priority), which is the order the
// gate evaluates them in — so the snapshot needs no sort of its own.
//
// LEFT JOIN on permissions because only a require_permission rule names one;
// an inner join would drop every public and deny rule.
var SnapshotRoutes = storm.SQL[RouteRow](`
SELECT rp.id::text AS id, rp.priority, rp.effect, rp.path_pattern,
       rp.host_pattern, rp.methods, rp.scope_bindings,
       p.key AS permission_key, a.slug AS application_slug
FROM route_policies rp
JOIN applications a ON a.id = rp.application_id
LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE rp.tenant_id = $1
ORDER BY a.slug, rp.priority`)

// TenantRow maps a slug to an id for the gate's request routing.
type TenantRow struct {
	ID   string
	Slug string
}

var SnapshotTenants = storm.SQL[TenantRow](`
SELECT id::text AS id, slug FROM tenants WHERE status = 'active'`)

// CatalogVersionRow is the invalidation counter the snapshot was built from.
type CatalogVersionRow struct {
	Version   int64
	ChangedAt time.Time
}

var SnapshotCatalogVersion = storm.SQL[CatalogVersionRow](`
SELECT version, changed_at FROM catalog_version WHERE tenant_id = $1`)

// SessionIDRow is one revoked session.
type SessionIDRow struct {
	ID string
}

// SnapshotRevokedSessions is the revocation denylist, bounded by the longest
// access-token TTL: a revocation older than that cannot match a still-valid
// token, so carrying it would grow the snapshot forever for no decision it
// could change.
var SnapshotRevokedSessions = storm.SQL[SessionIDRow](`
SELECT id::text AS id FROM sessions
WHERE tenant_id = $1
  AND revoked_at IS NOT NULL
  AND revoked_at > now() - $2::text::interval`)
