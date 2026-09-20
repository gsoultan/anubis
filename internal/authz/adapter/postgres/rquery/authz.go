package authzrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// AuthorizeRow carries the engine's one-bit answer.
type AuthorizeRow struct {
	Allow bool
}

// Authorize is THE engine call. Semantics live in migrations/0013 (+0009
// gates); Go never re-implements them on the online path.
var Authorize = storm.SQL[AuthorizeRow](`
SELECT authorize($1, $2, $3, $4::jsonb) AS allow`)

// ExplainRow is the engine's narrated decision.
type ExplainRow struct {
	Detail string
}

// AuthorizeExplain narrates a decision for the debugging screens.
var AuthorizeExplain = storm.SQL[ExplainRow](`
SELECT authorize_explain($1, $2, $3, $4::jsonb)::text AS detail`)

// PermissionMetaRow is the step-up metadata the middleware reads per request.
type PermissionMetaRow struct {
	ID           string
	Key          runtime.Null[string]
	Risk         string
	MinAssurance int16
	RequiresAmr  []string
	MaxAuthAge   string
	DeprecatedAt runtime.Null[time.Time]
}

// GetPermissionByKey feeds PermissionByKey; key is a generated column and
// nullable in the descriptor even though a hit always has one.
var GetPermissionByKey = storm.SQL[PermissionMetaRow](`
SELECT id::text AS id, key, risk, min_assurance, requires_amr,
       COALESCE(max_auth_age::text, '')::text AS max_auth_age, deprecated_at
FROM permissions
WHERE tenant_id = $1 AND key = $2`)

// RoleNameRow is one role name.
type RoleNameRow struct {
	Name string
}

// RolesForIdentity lists the distinct live-grant role names for an identity.
var RolesForIdentity = storm.SQL[RoleNameRow](`
SELECT DISTINCT r.name
FROM grants g
JOIN roles r ON r.id = g.role_id
WHERE g.identity_id = $1 AND g.tenant_id = $2
  AND g.revoked_at IS NULL AND g.valid_from <= now()
  AND (g.valid_until IS NULL OR g.valid_until > now())
ORDER BY r.name`)

// PermissionKeyRow is one effective permission key.
type PermissionKeyRow struct {
	Key runtime.Null[string]
}

// EffectivePermissionsForIdentity lists every live permission key an
// identity's grants confer.
var EffectivePermissionsForIdentity = storm.SQL[PermissionKeyRow](`
SELECT DISTINCT p.key
FROM grants g
JOIN role_permissions_effective rpe ON rpe.role_id = g.role_id
JOIN permissions p ON p.id = rpe.permission_id
WHERE g.identity_id = $1 AND g.tenant_id = $2
  AND g.revoked_at IS NULL AND g.valid_from <= now()
  AND (g.valid_until IS NULL OR g.valid_until > now())
  AND p.deprecated_at IS NULL
ORDER BY p.key`)

// EffectiveGrantRow is one live (grant, role, scope) triple for an identity.
//
// One row per SCOPE, not per grant: a grant carrying three scopes is three
// rows, and an UNSCOPED grant is one row with the scope columns null. Flattened
// rather than nested because the alternative is a second round trip per grant,
// and the caller reassembling by grant id costs nothing.
type EffectiveGrantRow struct {
	GrantID    string
	RoleID     string
	RoleName   string
	SelfScoped bool
	Axis       runtime.Null[string]
	NodeID     runtime.Null[string]
	Inherit    runtime.Null[bool]
	Exclude    runtime.Null[bool]

	// Permissions is what the grant's ROLE confers, already expanded through
	// role inheritance. Carried on the grant rather than left to a second call
	// because the expansion lives on the ADMIN plane (GetRoleEffective), and a
	// tenant-readable grant read whose permissions require a platform
	// credential answers nothing anybody can act on.
	Permissions []string
}

// EffectiveGrantsForIdentity lists an identity's live grants with their scopes.
//
// TENANT-SCOPED, and that is the entire reason it exists. The same question is
// answerable through AuthzAdminService/ListGrants, but the admin plane refuses
// any non-platform caller outright — so a service that needed a subject's
// grants had to hold a credential administering EVERY tenant in the
// installation to perform a read about one subject in one of them.
//
// `g.tenant_id = $2` is what makes that unnecessary, and it is not a filter
// added for tidiness: without it a tenant credential would read another
// tenant's grants, which is the whole reason the admin plane was closed.
//
// EXCLUSIONS ARE RETURNED. `mode` distinguishes a carve-out from an include,
// and a caller that ignores it builds a grant wider than the one held — see
// migration 0046.
//
// LEFT JOIN, so an unscoped grant appears rather than vanishing. An unscoped
// grant in Anubis means every node on every axis; dropping it here would
// silently narrow what a caller believes somebody holds.
var EffectiveGrantsForIdentity = storm.SQL[EffectiveGrantRow](`
SELECT g.id::text      AS grant_id,
       g.role_id::text AS role_id,
       r.name          AS role_name,
       g.self_scoped   AS self_scoped,
       gs.axis_code    AS axis,
       gs.scope_node_id::text AS node_id,
       gs.inherit      AS inherit,
       (gs.mode = 'exclude') AS exclude,
       COALESCE((SELECT array_agg(DISTINCT p.key ORDER BY p.key)
                   FROM role_permissions_effective rpe
                   JOIN permissions p ON p.id = rpe.permission_id
                  WHERE rpe.role_id = g.role_id
                    AND p.deprecated_at IS NULL), ARRAY[]::text[]) AS permissions
FROM grants g
JOIN roles r ON r.id = g.role_id
LEFT JOIN grant_scopes gs ON gs.grant_id = g.id
WHERE g.identity_id = $1 AND g.tenant_id = $2
  AND g.revoked_at IS NULL AND g.valid_from <= now()
  AND (g.valid_until IS NULL OR g.valid_until > now())
ORDER BY r.name, g.id, gs.axis_code, gs.scope_node_id`)

// ForestNodeRow is one node of the scope forest a PEP caches.
type ForestNodeRow struct {
	ID         string
	AxisCode   string
	ParentID   runtime.Null[string]
	ParentAxis runtime.Null[string]
	Name       string
}

// ScopeForestForTenant returns every live node on the requested axes.
//
// TENANT-SCOPED, for the same reason as EffectiveGrantsForIdentity: the forest
// is what a PEP needs to decide whether a grant at org:17 reaches a resource in
// unit:42, and reading it through ScopeAdminService/ListScopeNodes required a
// credential administering every tenant in the installation.
//
// The PARENT'S AXIS, not only its id. The forest crosses axes, so a consumer
// holding a parent id alone cannot place the parent without reading every axis
// and looking it up.
//
// Unpaged, deliberately: a PEP needs the WHOLE forest or none of it. A partial
// one is not a smaller forest, it is a different one — a node whose parent is
// missing has no ancestry, so a grant above it stops reaching it. The bound is
// the caller's axis list, and an axis with hundreds of thousands of nodes is
// one nobody should be caching in a PEP anyway.
var ScopeForestForTenant = storm.SQL[ForestNodeRow](`
SELECT n.id::text        AS id,
       n.axis_code       AS axis_code,
       n.parent_id::text AS parent_id,
       p.axis_code       AS parent_axis,
       n.name            AS name
FROM scope_nodes n
LEFT JOIN scope_nodes p ON p.id = n.parent_id
WHERE n.tenant_id = $1
  AND n.axis_code = ANY($2::text[])
  AND n.status = 'active'
ORDER BY n.axis_code, n.id`)

// StrictSimRow is the strict dry-run verdict.
type StrictSimRow struct {
	Allow bool
}

// AuthorizeStrictSim is the 0046 decision with ONE axis hypothetically flipped
// to default_effect='deny', so the report can be produced without touching
// scope_axes. Kept textually parallel to migrations/0046 — if that file
// changes, this must change with it (the integration suite asserts parity for
// the axis-unchanged case). $1 identity, $2 tenant, $3 permission (nullable),
// $4 targets jsonb, $5 strict axis.
//
// The dry run is the report an operator reads before making an axis strict,
// so it must count an exclusion the way the live engine will. Left on 0013's
// includes-only aggregate it would have reported allows that the real
// authorize() denies — the one number the report exists to get right.
var AuthorizeStrictSim = storm.SQL[StrictSimRow](`
WITH targets AS MATERIALIZED (
    SELECT t.key AS axis_code, t.value::uuid AS node_id
      FROM jsonb_each_text($4::jsonb) AS t(key, value)
     WHERE t.key NOT LIKE '\_%'
),
candidates AS (
    SELECT g.id, g.self_scoped
      FROM grants g
      JOIN identities i
        ON i.id = g.identity_id AND i.tenant_id = g.tenant_id
      JOIN role_permissions_effective rpe ON rpe.role_id = g.role_id
      JOIN permissions p ON p.id = rpe.permission_id
     WHERE g.identity_id = $1
       AND g.tenant_id   = $2
       AND g.revoked_at IS NULL
       AND g.valid_from <= now()
       AND (g.valid_until IS NULL OR g.valid_until > now())
       AND i.status = 'active'
       AND i.disabled_at IS NULL
       AND i.anonymized_at IS NULL
       AND p.tenant_id = $2
       AND p.key = $3
       AND p.deprecated_at IS NULL
       AND p.min_assurance <= i.assurance_level
),
axis_eval AS (
    -- 0 an exclude covers the target, 1 an include covers it, 2 silent.
    -- min = 1 is "included and not excluded". See migrations/0046.
    SELECT gs.grant_id, gs.axis_code,
           min(CASE WHEN NOT EXISTS (SELECT 1
                         FROM targets t
                         JOIN scope_closure c
                           ON c.descendant_id = t.node_id
                          AND c.ancestor_id   = gs.scope_node_id
                        WHERE t.axis_code = gs.axis_code
                          AND (gs.inherit OR c.depth = 0))   THEN 2
                    WHEN gs.mode = 'exclude'                 THEN 0
                    ELSE                                          1
               END) = 1 AS satisfied
      FROM grant_scopes gs JOIN candidates cd ON cd.id = gs.grant_id
     GROUP BY gs.grant_id, gs.axis_code
)
SELECT EXISTS (
    SELECT 1 FROM candidates cd
     WHERE (NOT cd.self_scoped
            OR ($4::jsonb ? '_owner'
                AND ($4::jsonb->>'_owner')::uuid = $1))
       AND NOT EXISTS (SELECT 1 FROM axis_eval ae
                        WHERE ae.grant_id = cd.id AND NOT ae.satisfied)
       AND NOT EXISTS (SELECT 1 FROM scope_axes a
                        WHERE ((a.default_effect = 'deny' AND a.status = 'active')
                               OR a.code = $5)
                          AND NOT EXISTS (SELECT 1 FROM grant_scopes gs2
                                           WHERE gs2.grant_id = cd.id
                                             AND gs2.axis_code = a.code))
)::boolean AS allow`)

// DecisionDetailRow is one audited decision's snapshotted inputs.
type DecisionDetailRow struct {
	Detail runtime.JSON
}

// SampleAuthorizeDecisions returns recent allow decisions with their
// snapshotted inputs, for strict dry-run replay. The audit writer records
// {subject, permission, targets} in detail.
var SampleAuthorizeDecisions = storm.SQL[DecisionDetailRow](`
SELECT detail
FROM audit_log
WHERE tenant_id = $1
  AND action = 'authorize' AND result = 'allow'
ORDER BY occurred_at DESC
LIMIT $2`)
