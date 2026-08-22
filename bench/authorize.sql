-- The multi-axis authorization decision, fail-closed.
CREATE OR REPLACE FUNCTION authorize(
    p_identity uuid, p_tenant uuid, p_permission text, p_targets jsonb
) RETURNS boolean LANGUAGE sql STABLE AS $fn$
WITH targets AS (
    SELECT key AS axis_code, value::uuid AS node_id
      FROM jsonb_each_text(p_targets)
),
candidates AS (
    SELECT g.id
      FROM grants g
      JOIN role_permissions_effective rpe ON rpe.role_id = g.role_id
      JOIN permissions p ON p.id = rpe.permission_id
     WHERE g.identity_id = p_identity
       AND g.tenant_id   = p_tenant
       AND g.revoked_at IS NULL
       AND g.valid_from <= now()
       AND (g.valid_until IS NULL OR g.valid_until > now())
       AND p.tenant_id = p_tenant
       AND p.key = p_permission
       AND p.deprecated_at IS NULL
),
-- One row per (grant, constrained axis). LEFT JOINs are load-bearing: an
-- INNER JOIN drops rows when the caller supplies no target for an axis the
-- grant constrains, which makes the axis vanish from evaluation and the
-- grant pass unchecked. That is a fail-OPEN bug, and the happy path never
-- exposes it because callers normally do send every axis.
axis_eval AS (
    SELECT gs.grant_id, gs.axis_code,
           bool_or(t.node_id IS NOT NULL AND c.descendant_id IS NOT NULL) AS satisfied
      FROM grant_scopes gs
      JOIN candidates cd ON cd.id = gs.grant_id
      LEFT JOIN targets t ON t.axis_code = gs.axis_code
      LEFT JOIN scope_closure c
             ON c.ancestor_id = gs.scope_node_id
            AND c.descendant_id = t.node_id
            AND (gs.inherit OR c.depth = 0)
     GROUP BY gs.grant_id, gs.axis_code
)
SELECT EXISTS (
    SELECT 1 FROM candidates cd
     WHERE NOT EXISTS (SELECT 1 FROM axis_eval ae
                        WHERE ae.grant_id = cd.id AND NOT ae.satisfied)
       AND NOT EXISTS (SELECT 1 FROM scope_axes a
                        WHERE a.default_effect = 'deny' AND a.status = 'active'
                          AND NOT EXISTS (SELECT 1 FROM grant_scopes gs2
                                           WHERE gs2.grant_id = cd.id
                                             AND gs2.axis_code = a.code))
);
$fn$;
