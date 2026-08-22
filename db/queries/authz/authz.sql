-- name: Authorize :one
-- The engine. Semantics live in migrations/0013 (+0009 gates); Go never
-- re-implements them on the online path.
SELECT authorize(sqlc.arg(identity_id), sqlc.arg(tenant_id),
                 sqlc.arg(permission), sqlc.arg(targets)::jsonb) AS allow;

-- name: AuthorizeExplain :one
SELECT authorize_explain(sqlc.arg(identity_id), sqlc.arg(tenant_id),
                         sqlc.arg(permission), sqlc.arg(targets)::jsonb)::text AS detail;

-- name: GetPermissionByKey :one
SELECT id, key, risk, min_assurance, requires_amr,
       COALESCE(max_auth_age::text, '')::text AS max_auth_age, deprecated_at
FROM permissions
WHERE tenant_id = sqlc.arg(tenant_id) AND key = sqlc.arg(key);

-- name: RolesForIdentity :many
SELECT DISTINCT r.name
FROM grants g
JOIN roles r ON r.id = g.role_id
WHERE g.identity_id = sqlc.arg(identity_id) AND g.tenant_id = sqlc.arg(tenant_id)
  AND g.revoked_at IS NULL AND g.valid_from <= now()
  AND (g.valid_until IS NULL OR g.valid_until > now())
ORDER BY r.name;

-- name: EffectivePermissionsForIdentity :many
SELECT DISTINCT p.key
FROM grants g
JOIN role_permissions_effective rpe ON rpe.role_id = g.role_id
JOIN permissions p ON p.id = rpe.permission_id
WHERE g.identity_id = sqlc.arg(identity_id) AND g.tenant_id = sqlc.arg(tenant_id)
  AND g.revoked_at IS NULL AND g.valid_from <= now()
  AND (g.valid_until IS NULL OR g.valid_until > now())
  AND p.deprecated_at IS NULL
ORDER BY p.key;

-- name: AuthorizeStrictSim :one
-- StrictDryRun support: the 0013 decision with ONE axis hypothetically flipped
-- to default_effect='deny', so the report can be produced without touching
-- scope_axes. Kept textually parallel to migrations/0013 — if that file
-- changes, this must change with it (the integration suite asserts parity for
-- the axis-unchanged case).
WITH targets AS MATERIALIZED (
    SELECT t.key AS axis_code, t.value::uuid AS node_id
      FROM jsonb_each_text(sqlc.arg(targets)::jsonb) AS t(key, value)
     WHERE t.key NOT LIKE '\_%'
),
candidates AS (
    SELECT g.id, g.self_scoped
      FROM grants g
      JOIN identities i
        ON i.id = g.identity_id AND i.tenant_id = g.tenant_id
      JOIN role_permissions_effective rpe ON rpe.role_id = g.role_id
      JOIN permissions p ON p.id = rpe.permission_id
     WHERE g.identity_id = sqlc.arg(identity_id)
       AND g.tenant_id   = sqlc.arg(tenant_id)
       AND g.revoked_at IS NULL
       AND g.valid_from <= now()
       AND (g.valid_until IS NULL OR g.valid_until > now())
       AND i.status = 'active'
       AND i.disabled_at IS NULL
       AND i.anonymized_at IS NULL
       AND p.tenant_id = sqlc.arg(tenant_id)
       AND p.key = sqlc.arg(permission)
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
            OR (sqlc.arg(targets)::jsonb ? '_owner'
                AND (sqlc.arg(targets)::jsonb->>'_owner')::uuid = sqlc.arg(identity_id)))
       AND NOT EXISTS (SELECT 1 FROM axis_eval ae
                        WHERE ae.grant_id = cd.id AND NOT ae.satisfied)
       AND NOT EXISTS (SELECT 1 FROM scope_axes a
                        WHERE ((a.default_effect = 'deny' AND a.status = 'active')
                               OR a.code = sqlc.arg(strict_axis))
                          AND NOT EXISTS (SELECT 1 FROM grant_scopes gs2
                                           WHERE gs2.grant_id = cd.id
                                             AND gs2.axis_code = a.code))
)::boolean AS allow;

-- name: SampleAuthorizeDecisions :many
-- Recent allow decisions with their snapshotted inputs, for strict dry-run
-- replay. The audit writer records {subject, permission, targets} in detail.
SELECT detail
FROM audit_log
WHERE tenant_id = sqlc.arg(tenant_id)
  AND action = 'authorize' AND result = 'allow'
ORDER BY occurred_at DESC
LIMIT sqlc.arg(sample_size);
