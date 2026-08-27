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

// StrictSimRow is the strict dry-run verdict.
type StrictSimRow struct {
	Allow bool
}

// AuthorizeStrictSim is the 0013 decision with ONE axis hypothetically flipped
// to default_effect='deny', so the report can be produced without touching
// scope_axes. Kept textually parallel to migrations/0013 — if that file
// changes, this must change with it (the integration suite asserts parity for
// the axis-unchanged case). $1 identity, $2 tenant, $3 permission (nullable),
// $4 targets jsonb, $5 strict axis.
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
    SELECT gs.grant_id, gs.axis_code,
           bool_or(EXISTS (SELECT 1
                     FROM targets t
                     JOIN scope_closure c
                       ON c.descendant_id = t.node_id
                      AND c.ancestor_id   = gs.scope_node_id
                    WHERE t.axis_code = gs.axis_code
                      AND (gs.inherit OR c.depth = 0))) AS satisfied
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
