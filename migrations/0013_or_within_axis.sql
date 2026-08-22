-- ============================================================================
-- 0013_or_within_axis.sql — restore OR semantics for multi-node axes
--
-- THE BUG: ADR-0004 defines "within an axis: OR — multiple nodes mean any of
-- them", and grant_scopes' primary key (grant_id, axis_code, scope_node_id)
-- was always built for it. But the 0007 performance rewrite (correlated
-- EXISTS) dropped the GROUP BY that the original bool_or formulation had, and
-- 0009 inherited the shape. Result: one satisfied-row PER NODE, and the
-- survival check "no unsatisfied row may exist" turned multiple nodes on one
-- axis into an accidental AND. A grant covering Jakarta OR Surabaya denied
-- both, because no target is under two sibling offices at once.
--
-- Why no test caught it: the seed attaches exactly one node per axis per
-- grant. Reproduced before this fix: target under the first of two granted
-- offices → false. The correctness suite now carries multi-node probes so the
-- OR semantics can never silently regress again.
--
-- THE FIX: aggregate per (grant, axis) with bool_or around the SAME
-- correlated-EXISTS probe — the planner-friendly per-row probe is unchanged,
-- it just votes into its axis group instead of alone.
-- ============================================================================

CREATE OR REPLACE FUNCTION authorize(
    p_identity uuid, p_tenant uuid, p_permission text, p_targets jsonb
) RETURNS boolean LANGUAGE sql STABLE PARALLEL SAFE AS $fn$
WITH targets AS MATERIALIZED (
    SELECT key AS axis_code, value::uuid AS node_id
      FROM jsonb_each_text(p_targets)
     WHERE key NOT LIKE '\_%'
),
candidates AS (
    SELECT g.id, g.self_scoped
      FROM grants g
      JOIN identities i
        ON i.id = g.identity_id AND i.tenant_id = g.tenant_id
      JOIN role_permissions_effective rpe ON rpe.role_id = g.role_id
      JOIN permissions p ON p.id = rpe.permission_id
     WHERE g.identity_id = p_identity
       AND g.tenant_id   = p_tenant
       AND g.revoked_at IS NULL
       AND g.valid_from <= now()
       AND (g.valid_until IS NULL OR g.valid_until > now())
       AND i.status = 'active'
       AND i.disabled_at IS NULL
       AND i.anonymized_at IS NULL
       AND p.tenant_id = p_tenant
       AND p.key = p_permission
       AND p.deprecated_at IS NULL
       AND p.min_assurance <= i.assurance_level
),
axis_eval AS (
    SELECT gs.grant_id, gs.axis_code,
           -- OR within the axis: any granted node covering the target wins.
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
            OR (p_targets ? '_owner'
                AND (p_targets->>'_owner')::uuid = p_identity))
       AND NOT EXISTS (SELECT 1 FROM axis_eval ae
                        WHERE ae.grant_id = cd.id AND NOT ae.satisfied)
       AND NOT EXISTS (SELECT 1 FROM scope_axes a
                        WHERE a.default_effect = 'deny' AND a.status = 'active'
                          AND NOT EXISTS (SELECT 1 FROM grant_scopes gs2
                                           WHERE gs2.grant_id = cd.id
                                             AND gs2.axis_code = a.code))
);
$fn$;
