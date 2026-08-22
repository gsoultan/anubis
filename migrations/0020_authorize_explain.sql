-- ============================================================================
-- 0020_authorize_explain.sql — the decision, decomposed
--
-- /v1/authorize/explain is not optional once more than two axes exist
-- (roadmap, phase 2). This function returns the full evaluation tree:
-- which grants carried the permission, which role conferred it (provenance
-- via role_permissions_effective.via_role_id), what each axis evaluated to
-- against which granted nodes, which strict axes a grant left unaddressed,
-- and — on denial — a stable machine reason plus the first failing axis.
--
-- PARITY RULE: 'allow' is computed by calling authorize() itself, never by
-- re-deriving it here. The decomposition can lag a semantics change; the
-- verdict cannot. The integration suite additionally asserts that the
-- decomposition agrees with the verdict on every probe.
--
-- Candidate grants here deliberately OMIT the identity-state and assurance
-- gates that authorize() applies, because "you hold the grant but your
-- identity is disabled" and "you hold the grant but your assurance level is
-- too low" are exactly what an operator needs to see. The gates are reported
-- separately under 'identity'.
-- ============================================================================

CREATE OR REPLACE FUNCTION authorize_explain(
    p_identity uuid, p_tenant uuid, p_permission text, p_targets jsonb
) RETURNS jsonb LANGUAGE sql STABLE AS $fn$
WITH targets AS MATERIALIZED (
    SELECT key AS axis_code, value::uuid AS node_id
      FROM jsonb_each_text(p_targets)
     WHERE key NOT LIKE '\_%'
),
verdict AS (
    SELECT authorize(p_identity, p_tenant, p_permission, p_targets) AS allow
),
ident AS (
    SELECT i.status, i.disabled_at, i.anonymized_at, i.assurance_level
      FROM identities i
     WHERE i.id = p_identity AND i.tenant_id = p_tenant
),
perm AS (
    SELECT p.id, p.min_assurance, p.deprecated_at, p.requires_amr,
           p.max_auth_age::text AS max_auth_age, p.risk
      FROM permissions p
     WHERE p.tenant_id = p_tenant AND p.key = p_permission
),
cand AS (
    SELECT g.id, g.self_scoped, r.name AS role_name, vr.name AS via_role_name
      FROM grants g
      JOIN role_permissions_effective rpe ON rpe.role_id = g.role_id
      JOIN perm p  ON p.id = rpe.permission_id
      JOIN roles r  ON r.id = g.role_id
      JOIN roles vr ON vr.id = rpe.via_role_id
     WHERE g.identity_id = p_identity
       AND g.tenant_id   = p_tenant
       AND g.revoked_at IS NULL
       AND g.valid_from <= now()
       AND (g.valid_until IS NULL OR g.valid_until > now())
),
axis_eval AS (
    SELECT gs.grant_id, gs.axis_code,
           bool_or(EXISTS (SELECT 1
                     FROM targets t
                     JOIN scope_closure c
                       ON c.descendant_id = t.node_id
                      AND c.ancestor_id   = gs.scope_node_id
                    WHERE t.axis_code = gs.axis_code
                      AND (gs.inherit OR c.depth = 0))) AS satisfied,
           jsonb_agg(jsonb_build_object(
               'node_id', gs.scope_node_id,
               'node',    sn.name,
               'inherit', gs.inherit) ORDER BY sn.name) AS nodes
      FROM grant_scopes gs
      JOIN cand cd ON cd.id = gs.grant_id
      JOIN scope_nodes sn ON sn.id = gs.scope_node_id
     GROUP BY gs.grant_id, gs.axis_code
),
strict_axes AS (
    SELECT code, sort_order FROM scope_axes
     WHERE default_effect = 'deny' AND status = 'active'
),
per_grant AS (
    SELECT cd.id, cd.self_scoped, cd.role_name, cd.via_role_name,
           (NOT cd.self_scoped
            OR (p_targets ? '_owner'
                AND (p_targets->>'_owner')::uuid = p_identity)) AS self_ok,
           NOT EXISTS (SELECT 1 FROM axis_eval ae
                        WHERE ae.grant_id = cd.id AND NOT ae.satisfied) AS axes_ok,
           COALESCE((SELECT jsonb_agg(jsonb_build_object(
                         'axis', ae.axis_code,
                         'satisfied', ae.satisfied,
                         'nodes', ae.nodes) ORDER BY ae.axis_code)
                       FROM axis_eval ae WHERE ae.grant_id = cd.id),
                    '[]'::jsonb) AS axes,
           COALESCE((SELECT jsonb_agg(sa.code ORDER BY sa.sort_order, sa.code)
                       FROM strict_axes sa
                      WHERE NOT EXISTS (SELECT 1 FROM grant_scopes gs2
                                         WHERE gs2.grant_id = cd.id
                                           AND gs2.axis_code = sa.code)),
                    '[]'::jsonb) AS strict_missing
      FROM cand cd
),
gates AS (
    SELECT EXISTS (SELECT 1 FROM ident) AS identity_found,
           COALESCE((SELECT status = 'active'
                        AND disabled_at IS NULL
                        AND anonymized_at IS NULL FROM ident), false) AS identity_ok,
           EXISTS (SELECT 1 FROM perm) AS permission_found,
           COALESCE((SELECT deprecated_at IS NULL FROM perm), false) AS permission_live,
           COALESCE((SELECT p.min_assurance <= i.assurance_level
                       FROM perm p, ident i), false) AS assurance_ok
)
SELECT jsonb_build_object(
    'allow', (SELECT allow FROM verdict),
    'identity', (SELECT jsonb_build_object(
        'found', identity_found, 'active', identity_ok,
        'assurance_ok', assurance_ok,
        'assurance_level', (SELECT assurance_level FROM ident)) FROM gates),
    'permission', jsonb_build_object(
        'found', (SELECT permission_found FROM gates),
        'live',  (SELECT permission_live  FROM gates),
        'risk',          (SELECT risk FROM perm),
        'min_assurance', (SELECT min_assurance FROM perm),
        'requires_amr',  (SELECT to_jsonb(requires_amr) FROM perm),
        'max_auth_age',  (SELECT max_auth_age FROM perm)),
    'grants', COALESCE((SELECT jsonb_agg(jsonb_build_object(
        'grant_id',       pg.id,
        'role',           pg.role_name,
        'via_role',       pg.via_role_name,
        'self_scoped',    pg.self_scoped,
        'self_ok',        pg.self_ok,
        'axes',           pg.axes,
        'axes_ok',        pg.axes_ok,
        'strict_missing', pg.strict_missing,
        'allowed', (pg.self_ok AND pg.axes_ok
                    AND pg.strict_missing = '[]'::jsonb))
        ORDER BY pg.role_name) FROM per_grant pg), '[]'::jsonb),
    'reason', CASE
        WHEN (SELECT allow FROM verdict) THEN NULL
        WHEN NOT (SELECT identity_found FROM gates) THEN 'identity_not_found'
        WHEN NOT (SELECT identity_ok    FROM gates) THEN 'identity_inactive'
        WHEN NOT (SELECT permission_found AND permission_live FROM gates)
             THEN 'permission_unknown'
        WHEN NOT EXISTS (SELECT 1 FROM cand) THEN 'permission_not_held'
        WHEN NOT (SELECT assurance_ok FROM gates) THEN 'insufficient_assurance'
        WHEN EXISTS (SELECT 1 FROM per_grant WHERE NOT self_ok)
             AND NOT EXISTS (SELECT 1 FROM per_grant WHERE self_ok)
             THEN 'self_scope_required'
        WHEN EXISTS (SELECT 1 FROM per_grant pg
                      WHERE pg.self_ok AND pg.axes_ok
                        AND pg.strict_missing <> '[]'::jsonb)
             THEN 'axis_unresolved'
        ELSE 'scope_mismatch' END,
    'failing_axis', CASE WHEN (SELECT allow FROM verdict) THEN NULL ELSE
        (SELECT ae.axis_code
           FROM axis_eval ae
           JOIN scope_axes a ON a.code = ae.axis_code
          WHERE NOT ae.satisfied
          ORDER BY a.sort_order, ae.axis_code
          LIMIT 1) END
);
$fn$;
