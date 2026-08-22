-- ============================================================================
-- 0009_authorize_realms.sql — authorization for external populations
--
-- Adds three gates on top of 0007, all fail-closed:
--
--   1. IDENTITY STATE   disabled, locked or anonymised identities are denied
--                       regardless of grants. Deprovisioning must not depend on
--                       someone remembering to revoke every grant.
--
--   2. ASSURANCE        permissions may demand a minimum identity assurance
--                       level. A self-registered applicant cannot approve a
--                       purchase order even if a misconfigured grant says so.
--                       Defence in depth against grant-administration error --
--                       exactly the class of mistake delegated administration
--                       makes more likely.
--
--   3. SELF-SCOPE       "you may read applications, but only your own." The
--                       dominant external-user shape, and not a scope question:
--                       no tree node describes "the row you own". The caller
--                       passes the record owner as the reserved target key
--                       '_owner'. If a grant is self_scoped and no '_owner' is
--                       supplied, the answer is DENY -- same fail-closed rule
--                       that governs unresolved axes.
-- ============================================================================

CREATE OR REPLACE FUNCTION authorize(
    p_identity uuid, p_tenant uuid, p_permission text, p_targets jsonb
) RETURNS boolean LANGUAGE sql STABLE PARALLEL SAFE AS $fn$
WITH targets AS MATERIALIZED (
    SELECT key AS axis_code, value::uuid AS node_id
      FROM jsonb_each_text(p_targets)
     -- reserved keys start with '_'; scope_axes.code CHECK forbids that prefix,
     -- so an axis can never collide with one.
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
       -- gate 1: identity state
       AND i.status = 'active'
       AND i.disabled_at IS NULL
       AND i.anonymized_at IS NULL
       AND p.tenant_id = p_tenant
       AND p.key = p_permission
       AND p.deprecated_at IS NULL
       -- gate 2: assurance
       AND p.min_assurance <= i.assurance_level
),
axis_eval AS (
    SELECT gs.grant_id, gs.axis_code,
           EXISTS (SELECT 1
                     FROM targets t
                     JOIN scope_closure c
                       ON c.descendant_id = t.node_id
                      AND c.ancestor_id   = gs.scope_node_id
                    WHERE t.axis_code = gs.axis_code
                      AND (gs.inherit OR c.depth = 0)) AS satisfied
      FROM grant_scopes gs JOIN candidates cd ON cd.id = gs.grant_id
)
SELECT EXISTS (
    SELECT 1 FROM candidates cd
     -- gate 3: self-scope. Fail-closed when '_owner' is absent.
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
