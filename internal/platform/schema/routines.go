package platformschema

// The SQL-bodied objects: the authorization engine, the triggers that keep the
// derived tables true, and the one view.
//
// These are the part of the schema storm does not build from Go, and the part
// anubis most depends on: authorize() and authorize_explain() ARE the decision,
// the membership_* family fans an assignment out, and the statement triggers
// maintain role_permissions_effective, scope_closure and catalog_version. A
// schema without them is a schema the application cannot run against, which is
// why a model that could not express them could not own this schema.
//
// Bodies are verbatim text because PL/pgSQL is a language storm does not parse.
// What storm owns is the lifecycle — creation order, change detection against
// the body PostgreSQL actually stored, and the drop-then-create a trigger needs
// so one somebody had disabled does not come back disabled.

import "github.com/gsoultan/storm"

// routines returns every function, view and trigger, in dependency order:
// functions first (a view may call one), then the view, then the triggers,
// which need both their table and the function they execute.
func routines() []any {
	return []any{
		storm.Function("authorize", "p_identity uuid, p_tenant uuid, p_permission text, p_targets jsonb", "boolean").Language("sql").Stable().Body(`
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
    -- OR within the axis over the includes; any exclude covering the target
    -- vetoes the axis for THIS grant and no other.
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
`),
		storm.Function("authorize_explain", "p_identity uuid, p_tenant uuid, p_permission text, p_targets jsonb", "jsonb").Language("sql").Stable().Body(`
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
axis_probe AS (
    SELECT gs.grant_id, gs.axis_code, gs.mode, gs.inherit,
           gs.scope_node_id, sn.name AS node_name,
           EXISTS (SELECT 1
                     FROM targets t
                     JOIN scope_closure c
                       ON c.descendant_id = t.node_id
                      AND c.ancestor_id   = gs.scope_node_id
                    WHERE t.axis_code = gs.axis_code
                      AND (gs.inherit OR c.depth = 0)) AS covers
      FROM grant_scopes gs
      JOIN cand cd ON cd.id = gs.grant_id
      JOIN scope_nodes sn ON sn.id = gs.scope_node_id
),
axis_eval AS (
    SELECT grant_id, axis_code,
           bool_or(mode = 'include' AND covers) AS included,
           bool_or(mode = 'exclude' AND covers) AS excluded,
           bool_or(mode = 'include' AND covers)
             AND NOT bool_or(mode = 'exclude' AND covers) AS satisfied,
           jsonb_agg(jsonb_build_object(
               'node_id', scope_node_id,
               'node',    node_name,
               'inherit', inherit,
               'mode',    mode) ORDER BY mode DESC, node_name) AS nodes
      FROM axis_probe
     GROUP BY grant_id, axis_code
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
                         'included', ae.included,
                         'excluded', ae.excluded,
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
        -- An exclusion only EXPLAINS the denial where it decided it: the
        -- include reached the target and the exclude took it back. Where
        -- nothing included the target the grant was never going to allow,
        -- and calling that an exclusion would send an operator to edit the
        -- wrong half of the grant.
        WHEN EXISTS (SELECT 1 FROM axis_eval ae
                      WHERE ae.included AND ae.excluded)
             THEN 'scope_excluded'
        ELSE 'scope_mismatch' END,
    'failing_axis', CASE WHEN (SELECT allow FROM verdict) THEN NULL ELSE
        (SELECT ae.axis_code
           FROM axis_eval ae
           JOIN scope_axes a ON a.code = ae.axis_code
          WHERE NOT ae.satisfied
          ORDER BY a.sort_order, ae.axis_code
          LIMIT 1) END
);
`),
		storm.Function("bump_catalog_version", "p_tenant uuid", "void").Body(`
BEGIN
    INSERT INTO catalog_version (tenant_id, version, changed_at)
         VALUES (p_tenant, 1, now())
    ON CONFLICT (tenant_id) DO UPDATE
            SET version = catalog_version.version + 1,
                changed_at = now();
    -- push invalidation; pollers act as the backstop when NOTIFY is dropped
    PERFORM pg_notify('anubis_catalog', p_tenant::text);
END;
`),
		storm.Function("ensure_month_partitions", "p_table text, p_col text, p_months_ahead integer DEFAULT 3", "void").Body(`
DECLARE
    i int; v_start date; v_end date; v_name text;
BEGIN
    FOR i IN 0..p_months_ahead LOOP
        v_start := date_trunc('month', now())::date + (i || ' month')::interval;
        v_end   := v_start + interval '1 month';
        v_name  := format('%s_%s', p_table, to_char(v_start, 'YYYYMM'));
        IF to_regclass(v_name) IS NULL THEN
            EXECUTE format(
              'CREATE TABLE %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
              v_name, p_table, v_start, v_end);
        END IF;
    END LOOP;
END;
`),
		storm.Function("membership_assign", "p_identity uuid, p_membership uuid, p_by uuid", "integer").Body(`
DECLARE
    v_tenant uuid;
    v_n int := 0;
    e record;
    v_grant uuid;
BEGIN
    SELECT tenant_id INTO v_tenant FROM memberships WHERE id = p_membership;
    IF v_tenant IS NULL THEN RAISE EXCEPTION 'unknown membership %', p_membership; END IF;

    INSERT INTO membership_members (membership_id, identity_id, tenant_id, assigned_by)
    VALUES (p_membership, p_identity, v_tenant, p_by)
    ON CONFLICT DO NOTHING;
    IF NOT FOUND THEN RETURN 0; END IF;   -- already a member: no duplicate fan-out

    FOR e IN SELECT id, role_id FROM membership_entries WHERE membership_id = p_membership LOOP
        -- realm-guard trigger fires HERE, per row: wrong population aborts all
        INSERT INTO grants (tenant_id, identity_id, role_id, granted_by,
                            via_membership_id, via_entry_id)
        VALUES (v_tenant, p_identity, e.role_id, p_by, p_membership, e.id)
        RETURNING id INTO v_grant;

        INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id, inherit, mode)
        SELECT v_grant, tenant_id, axis_code, scope_node_id, inherit, mode
          FROM membership_entry_scopes WHERE entry_id = e.id;
        v_n := v_n + 1;
    END LOOP;
    RETURN v_n;
END;
`),
		storm.Function("membership_resync", "p_membership uuid", "integer").Body(`
DECLARE
    v_tenant uuid; m record; e record; v_grant uuid; v_n int := 0;
BEGIN
    SELECT tenant_id INTO v_tenant FROM memberships WHERE id = p_membership;
    UPDATE grants g SET revoked_at = now()
     WHERE g.via_membership_id = p_membership AND g.revoked_at IS NULL
       AND NOT EXISTS (SELECT 1 FROM membership_entries me WHERE me.id = g.via_entry_id);
    GET DIAGNOSTICS v_n = ROW_COUNT;

    FOR m IN SELECT identity_id FROM membership_members WHERE membership_id = p_membership LOOP
        FOR e IN SELECT id, role_id FROM membership_entries me
                  WHERE me.membership_id = p_membership
                    AND NOT EXISTS (SELECT 1 FROM grants g
                                     WHERE g.via_entry_id = me.id
                                       AND g.identity_id = m.identity_id
                                       AND g.revoked_at IS NULL) LOOP
            INSERT INTO grants (tenant_id, identity_id, role_id, granted_by,
                                via_membership_id, via_entry_id)
            VALUES (v_tenant, m.identity_id, e.role_id, m.identity_id, p_membership, e.id)
            RETURNING id INTO v_grant;
            INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id, inherit, mode)
            SELECT v_grant, tenant_id, axis_code, scope_node_id, inherit, mode
              FROM membership_entry_scopes WHERE entry_id = e.id;
            v_n := v_n + 1;
        END LOOP;
    END LOOP;
    RETURN v_n;
END;
`),
		storm.Function("membership_unassign", "p_identity uuid, p_membership uuid", "integer").Body(`
DECLARE v_n int;
BEGIN
    UPDATE grants SET revoked_at = now()
     WHERE identity_id = p_identity AND via_membership_id = p_membership
       AND revoked_at IS NULL;
    GET DIAGNOSTICS v_n = ROW_COUNT;
    DELETE FROM membership_members
     WHERE membership_id = p_membership AND identity_id = p_identity;
    RETURN v_n;
END;
`),
		storm.Function("pii_shred", "p_key uuid, p_reason text", "boolean").Body(`
DECLARE v_tenant uuid;
BEGIN
    SELECT tenant_id INTO v_tenant FROM pii_keys WHERE id = p_key;
    IF v_tenant IS NULL THEN
        RETURN false;   -- already shredded; erasure is idempotent by design
    END IF;
    INSERT INTO pii_key_tombstones (key_id, tenant_id, reason)
         VALUES (p_key, v_tenant, p_reason)
    ON CONFLICT (key_id) DO NOTHING;
    DELETE FROM pii_keys WHERE id = p_key;
    RETURN true;
END;
`),
		storm.Function("role_recompute_effective", "p_role uuid", "void").Body(`
BEGIN
    DELETE FROM role_permissions_effective WHERE role_id = p_role;

    INSERT INTO role_permissions_effective (role_id, permission_id, via_role_id)
    WITH RECURSIVE ancestry AS (
        SELECT p_role AS rid
        UNION
        SELECT rp.parent_id FROM role_parents rp JOIN ancestry a ON a.rid = rp.role_id
    ) CYCLE rid SET is_cycle USING cyc_path
    SELECT DISTINCT ON (perm.id) p_role, perm.id, a.rid
      FROM ancestry a
      JOIN LATERAL (
            SELECT pm.id FROM role_permissions x
              JOIN permissions pm ON pm.id = x.permission_id
             WHERE x.role_id = a.rid AND pm.deprecated_at IS NULL
            UNION
            SELECT pm.id FROM role_permission_patterns pp
              JOIN permissions pm
                ON pm.key LIKE replace(pp.pattern, '*', '%')
             WHERE pp.role_id = a.rid AND pm.deprecated_at IS NULL
      ) perm ON true
     WHERE NOT a.is_cycle;
END;
`),
		storm.Function("scope_add_node", "p_tenant uuid, p_axis text, p_type text, p_parent uuid, p_slug text, p_name text, p_external_ref text DEFAULT NULL::text", "uuid").Body(`
DECLARE
    v_id uuid;
    v_parent_depth integer;
BEGIN
    -- The parent's own distance from its axis root. One indexed probe.
    SELECT max(depth) INTO v_parent_depth
      FROM scope_closure WHERE descendant_id = p_parent;

    IF v_parent_depth IS NOT NULL AND v_parent_depth + 1 > scope_max_depth() THEN
        RAISE EXCEPTION
            'scope hierarchy too deep: attaching %.% under % would reach depth %, limit is %',
            p_axis, p_slug, p_parent, v_parent_depth + 1, scope_max_depth()
            USING ERRCODE = 'program_limit_exceeded',
                  HINT = 'A chain this long is almost always a cycle or a self-referencing external feed, not a real hierarchy.';
    END IF;

    INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id,
                             slug, name, external_ref)
         VALUES (p_tenant, p_axis, p_type, p_parent, p_slug, p_name, p_external_ref)
      RETURNING id INTO v_id;

    INSERT INTO scope_closure (ancestor_id, descendant_id, depth)
    SELECT c.ancestor_id, v_id, c.depth + 1
      FROM scope_closure c
     WHERE c.descendant_id = p_parent
     UNION ALL
    SELECT v_id, v_id, 0;

    RETURN v_id;
END;
`),
		storm.Function("scope_ensure_root", "p_tenant uuid, p_axis text", "uuid").Body(`
DECLARE
    v_id uuid;
    v_type text;
BEGIN
    SELECT id INTO v_id FROM scope_nodes
     WHERE tenant_id = p_tenant AND axis_code = p_axis AND is_axis_root;
    IF FOUND THEN RETURN v_id; END IF;

    -- The root type is the one with NO legal parents. Picking alphabetically
    -- silently produced roots typed 'department' on the org axis.
    SELECT code INTO v_type FROM scope_node_types
     WHERE axis_code = p_axis AND cardinality(parent_types) = 0
     ORDER BY code LIMIT 1;
    IF v_type IS NULL THEN
        RAISE EXCEPTION 'axis % has no root node type (none with empty parent_types)', p_axis;
    END IF;

    INSERT INTO scope_nodes (tenant_id, axis_code, node_type, slug, name, is_axis_root)
         VALUES (p_tenant, p_axis, v_type, '_root', 'All ' || p_axis, true)
      RETURNING id INTO v_id;

    INSERT INTO scope_closure (ancestor_id, descendant_id, depth)
         VALUES (v_id, v_id, 0);
    RETURN v_id;
END;
`),
		storm.Function("scope_max_depth", "", "integer").Language("sql").Immutable().Body(`
    SELECT 32767;   -- the smallint ceiling on scope_closure.depth
`),
		storm.Function("scope_move_node", "p_node uuid, p_new_parent uuid", "void").Body(`
DECLARE
    v_new_parent_depth integer;
    v_subtree_depth    integer;
BEGIN
    IF EXISTS (SELECT 1 FROM scope_closure
                WHERE ancestor_id = p_node AND descendant_id = p_new_parent) THEN
        RAISE EXCEPTION 'cycle: % is inside the subtree of %', p_new_parent, p_node;
    END IF;

    -- Deepest resulting node = new parent's depth + 1 + the subtree's own
    -- height. Both halves can be legal while the graft is not.
    SELECT max(depth) INTO v_new_parent_depth
      FROM scope_closure WHERE descendant_id = p_new_parent;
    SELECT max(depth) INTO v_subtree_depth
      FROM scope_closure WHERE ancestor_id = p_node;

    IF COALESCE(v_new_parent_depth, 0) + COALESCE(v_subtree_depth, 0) + 1 > scope_max_depth() THEN
        RAISE EXCEPTION
            'scope hierarchy too deep: moving % under % would reach depth %, limit is %',
            p_node, p_new_parent,
            COALESCE(v_new_parent_depth, 0) + COALESCE(v_subtree_depth, 0) + 1,
            scope_max_depth()
            USING ERRCODE = 'program_limit_exceeded';
    END IF;

    -- sever links from former ancestors into the moving subtree
    DELETE FROM scope_closure
     WHERE descendant_id IN (SELECT descendant_id FROM scope_closure WHERE ancestor_id = p_node)
       AND ancestor_id   IN (SELECT ancestor_id  FROM scope_closure
                              WHERE descendant_id = p_node AND ancestor_id <> p_node);

    -- graft onto new ancestors
    INSERT INTO scope_closure (ancestor_id, descendant_id, depth)
    SELECT sup.ancestor_id, sub.descendant_id, sup.depth + sub.depth + 1
      FROM scope_closure sup
      CROSS JOIN scope_closure sub
     WHERE sup.descendant_id = p_new_parent
       AND sub.ancestor_id   = p_node;

    UPDATE scope_nodes SET parent_id = p_new_parent, updated_at = now()
     WHERE id = p_node;
END;
`),
		storm.Function("scope_sync_apply", "p_source uuid, p_rows jsonb, p_dry boolean DEFAULT false", "jsonb").Body(`
DECLARE
    src record; root_id uuid; e jsonb;
    v_ref text; v_pref text; v_name text; v_type text;
    v_node record; v_parent uuid;
    n_add int := 0; n_ren int := 0; n_mov int := 0; n_arc int := 0; n_same int := 0;
    errs jsonb := '[]'; rep jsonb; run_id uuid;
    -- refs this dry run would have created; empty and unused when p_dry=false
    v_pending text[] := '{}';
BEGIN
    SELECT * INTO src FROM scope_sync_sources WHERE id = p_source;
    IF src IS NULL THEN RAISE EXCEPTION 'unknown sync source %', p_source; END IF;
    SELECT id INTO root_id FROM scope_nodes
     WHERE tenant_id = src.tenant_id AND axis_code = src.axis_code AND is_axis_root;

    INSERT INTO scope_sync_runs (source_id, dry, status)
    VALUES (p_source, p_dry, 'running') RETURNING id INTO run_id;

    FOR e IN SELECT * FROM jsonb_array_elements(p_rows) LOOP
        v_ref  := e->>'ref';
        v_pref := e->>'parent_ref';
        v_name := e->>'name';
        v_type := COALESCE(e->>'node_type', src.config->>'default_node_type');
        BEGIN
            -- resolve parent: by ref, or the axis root
            IF v_pref IS NULL THEN v_parent := root_id;
            ELSE
                SELECT id INTO v_parent FROM scope_nodes
                 WHERE tenant_id = src.tenant_id AND axis_code = src.axis_code
                   AND external_ref = v_pref;
                IF v_parent IS NULL THEN
                    IF p_dry AND v_pref = ANY (v_pending) THEN
                        -- It would exist by now in a real run. Stand in with
                        -- the root: dry mode writes nothing, so this only
                        -- affects which counter the row lands in, never data.
                        v_parent := root_id;
                    ELSE
                        RAISE EXCEPTION 'parent ref "%" not found (feed must list parents first)', v_pref;
                    END IF;
                END IF;
            END IF;

            SELECT * INTO v_node FROM scope_nodes
             WHERE tenant_id = src.tenant_id AND axis_code = src.axis_code
               AND external_ref = v_ref;

            IF v_node IS NULL THEN
                IF NOT p_dry THEN
                    PERFORM scope_add_node(src.tenant_id, src.axis_code, v_type, v_parent,
                        -- slug from name; ref-suffixed on sibling collision
                        left(regexp_replace(lower(v_name), '[^a-z0-9]+', '-', 'g'), 40)
                          || CASE WHEN EXISTS (SELECT 1 FROM scope_nodes
                               WHERE parent_id = v_parent
                                 AND slug = left(regexp_replace(lower(v_name), '[^a-z0-9]+', '-', 'g'), 40))
                             THEN '-' || lower(right(v_ref, 6)) ELSE '' END,
                        v_name, v_ref);
                ELSE
                    v_pending := array_append(v_pending, v_ref);
                END IF;
                n_add := n_add + 1;
            ELSE
                IF v_node.parent_id IS DISTINCT FROM v_parent THEN
                    IF NOT p_dry THEN PERFORM scope_move_node(v_node.id, v_parent); END IF;
                    n_mov := n_mov + 1;
                ELSIF v_node.name IS DISTINCT FROM v_name OR v_node.status = 'archived' THEN
                    IF NOT p_dry THEN
                        UPDATE scope_nodes
                           SET name = v_name, status = 'active', updated_at = now()
                         WHERE id = v_node.id;
                    END IF;
                    n_ren := n_ren + 1;
                ELSE
                    n_same := n_same + 1;
                END IF;
            END IF;
        EXCEPTION WHEN OTHERS THEN
            errs := errs || jsonb_build_object('ref', v_ref, 'error', SQLERRM);
        END;
    END LOOP;

    -- rows gone from the source: archive ONLY sync-owned (ref-carrying) nodes
    IF p_dry THEN
        SELECT count(*) INTO n_arc FROM scope_nodes n
         WHERE n.tenant_id = src.tenant_id AND n.axis_code = src.axis_code
           AND n.external_ref IS NOT NULL AND n.status = 'active'
           AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(p_rows) r
                            WHERE r->>'ref' = n.external_ref);
    ELSE
        WITH gone AS (
            UPDATE scope_nodes n SET status = 'archived', updated_at = now()
             WHERE n.tenant_id = src.tenant_id AND n.axis_code = src.axis_code
               AND n.external_ref IS NOT NULL AND n.status = 'active'
               AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(p_rows) r
                                WHERE r->>'ref' = n.external_ref)
            RETURNING 1)
        SELECT count(*) INTO n_arc FROM gone;
    END IF;

    rep := jsonb_build_object('added', n_add, 'renamed', n_ren, 'moved', n_mov,
        'archived', n_arc, 'unchanged', n_same, 'errors', errs, 'dry', p_dry);
    UPDATE scope_sync_runs
       SET finished_at = now(), report = rep,
           status = CASE WHEN p_dry THEN 'dry_run'
                         WHEN jsonb_array_length(errs) > 0 THEN 'failed'
                         ELSE 'ok' END
     WHERE id = run_id;
    IF NOT p_dry THEN
        UPDATE scope_sync_sources SET last_run_at = now() WHERE id = p_source;
    END IF;
    RETURN rep;
END;
`),
		storm.Function("trg_bump_catalog", "", "trigger").Body(`
DECLARE
    v_tenant uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        EXECUTE format('SELECT ($1).%I', TG_ARGV[0]) INTO v_tenant USING OLD;
    ELSE
        EXECUTE format('SELECT ($1).%I', TG_ARGV[0]) INTO v_tenant USING NEW;
    END IF;
    IF v_tenant IS NOT NULL THEN
        PERFORM bump_catalog_version(v_tenant);
    END IF;
    RETURN NULL;   -- AFTER trigger
END;
`),
		storm.Function("trg_bump_catalog_all_tenants", "", "trigger").Body(`
DECLARE r record;
BEGIN
    FOR r IN SELECT id FROM tenants LOOP
        PERFORM bump_catalog_version(r.id);
    END LOOP;
    RETURN NULL;
END;
`),
		storm.Function("trg_bump_catalog_applications_stmt", "", "trigger").Body(`
DECLARE r record;
BEGIN
    IF TG_OP = 'INSERT' THEN
        FOR r IN SELECT DISTINCT tenant_id FROM newtab LOOP
            PERFORM bump_catalog_version(r.tenant_id);
        END LOOP;
    ELSIF TG_OP = 'DELETE' THEN
        FOR r IN SELECT DISTINCT tenant_id FROM oldtab LOOP
            PERFORM bump_catalog_version(r.tenant_id);
        END LOOP;
    ELSE
        FOR r IN SELECT DISTINCT n.tenant_id
                   FROM newtab n JOIN oldtab o ON o.id = n.id
                  WHERE o.slug IS DISTINCT FROM n.slug
        LOOP
            PERFORM bump_catalog_version(r.tenant_id);
        END LOOP;
    END IF;
    RETURN NULL;
END;
`),
		storm.Function("trg_bump_catalog_identities_stmt", "", "trigger").Body(`
DECLARE r record;
BEGIN
    IF TG_OP = 'INSERT' THEN
        FOR r IN SELECT DISTINCT tenant_id FROM newtab LOOP
            PERFORM bump_catalog_version(r.tenant_id);
        END LOOP;
    ELSIF TG_OP = 'DELETE' THEN
        FOR r IN SELECT DISTINCT tenant_id FROM oldtab LOOP
            PERFORM bump_catalog_version(r.tenant_id);
        END LOOP;
    ELSE
        FOR r IN SELECT DISTINCT n.tenant_id
                   FROM newtab n JOIN oldtab o ON o.id = n.id
                  WHERE o.status          IS DISTINCT FROM n.status
                     OR o.token_epoch     IS DISTINCT FROM n.token_epoch
                     OR o.assurance_level IS DISTINCT FROM n.assurance_level
                     OR o.disabled_at     IS DISTINCT FROM n.disabled_at
                     OR o.anonymized_at   IS DISTINCT FROM n.anonymized_at
        LOOP
            PERFORM bump_catalog_version(r.tenant_id);
        END LOOP;
    END IF;
    RETURN NULL;
END;
`),
		storm.Function("trg_bump_catalog_sessions_stmt", "", "trigger").Body(`
DECLARE r record;
BEGIN
    FOR r IN SELECT DISTINCT n.tenant_id
               FROM newtab n JOIN oldtab o ON o.id = n.id
              WHERE o.revoked_at IS NULL AND n.revoked_at IS NOT NULL
    LOOP
        PERFORM bump_catalog_version(r.tenant_id);
    END LOOP;
    RETURN NULL;
END;
`),
		storm.Function("trg_bump_catalog_stmt", "", "trigger").Body(`
DECLARE
    r record;
BEGIN
    IF TG_OP = 'DELETE' THEN
        FOR r IN EXECUTE 'SELECT DISTINCT tenant_id FROM oldtab WHERE tenant_id IS NOT NULL' LOOP
            PERFORM bump_catalog_version(r.tenant_id);
        END LOOP;
    ELSE
        FOR r IN EXECUTE 'SELECT DISTINCT tenant_id FROM newtab WHERE tenant_id IS NOT NULL' LOOP
            PERFORM bump_catalog_version(r.tenant_id);
        END LOOP;
    END IF;
    RETURN NULL;
END;
`),
		storm.Function("trg_bump_catalog_via_role", "", "trigger").Body(`
DECLARE r record;
BEGIN
    IF TG_OP = 'DELETE' THEN
        FOR r IN SELECT DISTINCT ro.tenant_id
                   FROM oldtab o JOIN roles ro ON ro.id = o.role_id
        LOOP
            PERFORM bump_catalog_version(r.tenant_id);
        END LOOP;
    ELSE
        FOR r IN SELECT DISTINCT ro.tenant_id
                   FROM newtab n JOIN roles ro ON ro.id = n.role_id
        LOOP
            PERFORM bump_catalog_version(r.tenant_id);
        END LOOP;
    END IF;
    RETURN NULL;
END;
`),
		storm.Function("trg_entry_exclusion_needs_include", "", "trigger").Body(`
BEGIN
    IF NOT EXISTS (SELECT 1 FROM membership_entry_scopes mes
                    WHERE mes.entry_id  = NEW.entry_id
                      AND mes.axis_code = NEW.axis_code
                      AND mes.mode      = 'include') THEN
        RAISE EXCEPTION
            'membership entry % excludes a place on axis % without including one first',
            NEW.entry_id, NEW.axis_code USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
`),
		storm.Function("trg_exclusion_needs_include", "", "trigger").Body(`
BEGIN
    IF NOT EXISTS (SELECT 1 FROM grant_scopes gs
                    WHERE gs.grant_id  = NEW.grant_id
                      AND gs.axis_code = NEW.axis_code
                      AND gs.mode      = 'include') THEN
        RAISE EXCEPTION
            'grant % excludes a place on axis % without including one first',
            NEW.grant_id, NEW.axis_code USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
`),
		storm.Function("trg_grant_realm_guard", "", "trigger").Body(`
DECLARE
    v_kind    text;
    v_allowed text[];
    v_role    text;
BEGIN
    SELECT r.kind INTO v_kind
      FROM identities i JOIN realms r ON r.id = i.realm_id
     WHERE i.id = NEW.identity_id;

    -- identities predating realm assignment are unconstrained
    IF v_kind IS NULL THEN RETURN NEW; END IF;

    SELECT allowed_realm_kinds, name INTO v_allowed, v_role
      FROM roles WHERE id = NEW.role_id;

    IF NOT (v_kind = ANY (v_allowed)) THEN
        RAISE EXCEPTION
          'role "%" may not be granted to a "%" identity (allowed: %)',
          v_role, v_kind, v_allowed
          USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN NEW;
END;
`),
		storm.Function("trg_grant_role_live", "", "trigger").Body(`
DECLARE
    v_name text;
    v_when timestamptz;
BEGIN
    SELECT r.name, r.deprecated_at INTO v_name, v_when
      FROM roles r WHERE r.id = NEW.role_id;

    IF v_when IS NOT NULL THEN
        RAISE EXCEPTION
          'role "%" was retired from the catalog on % and cannot be granted; existing grants of it still work',
          v_name, v_when::date
          USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN NEW;
END;
`),
		storm.Function("trg_scope_node_type_guard", "", "trigger").Body(`
DECLARE
    v_parent_type text;
    v_legal       text[];
BEGIN
    -- Roots are covered by the existing CHECK pair (root ⇔ no parent).
    IF NEW.parent_id IS NULL THEN RETURN NEW; END IF;

    SELECT node_type INTO v_parent_type FROM scope_nodes WHERE id = NEW.parent_id;
    SELECT parent_types INTO v_legal FROM scope_node_types
     WHERE code = NEW.node_type AND axis_code = NEW.axis_code;

    IF NOT (v_parent_type = ANY (v_legal)) THEN
        RAISE EXCEPTION
          'a "%" may not sit under a "%" (legal parents: %)',
          NEW.node_type, v_parent_type, array_to_string(v_legal, ', ')
          USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
`),
		storm.Function("trg_seed_auth_pages", "", "trigger").Body(`
BEGIN
    INSERT INTO auth_pages (tenant_id, kind, slug, name, is_default, config)
    SELECT n.id, k.kind, 'default', k.label, true, '{}'::jsonb
      FROM newtab n
      CROSS JOIN (VALUES ('signin',  'Default sign-in'),
                         ('signout', 'Default sign-out')) AS k(kind, label)
    ON CONFLICT DO NOTHING;
    RETURN NULL;
END;
`),
		storm.Function("trg_self_scope_guard", "", "trigger").Body(`
BEGIN
    IF EXISTS (SELECT 1 FROM grants g
                WHERE g.id = NEW.grant_id AND g.self_scoped) THEN
        RAISE EXCEPTION 'self-scoped grant % may not carry axis constraints',
              NEW.grant_id USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
`),
		storm.View("signin_pages", `SELECT tenant_id,
    config,
    updated_at
   FROM auth_pages
  WHERE kind = 'signin'::text AND is_default`),
		storm.Trigger("bump_applications_del", "applications", `CREATE TRIGGER bump_applications_del AFTER DELETE ON applications REFERENCING OLD TABLE AS oldtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_applications_stmt()`),
		storm.Trigger("bump_applications_ins", "applications", `CREATE TRIGGER bump_applications_ins AFTER INSERT ON applications REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_applications_stmt()`),
		storm.Trigger("bump_applications_slug", "applications", `CREATE TRIGGER bump_applications_slug AFTER UPDATE ON applications REFERENCING OLD TABLE AS oldtab NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_applications_stmt()`),
		storm.Trigger("grant_scopes_bump_del", "grant_scopes", `CREATE TRIGGER grant_scopes_bump_del AFTER DELETE ON grant_scopes REFERENCING OLD TABLE AS oldtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("grant_scopes_bump_ins", "grant_scopes", `CREATE TRIGGER grant_scopes_bump_ins AFTER INSERT ON grant_scopes REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("grant_scopes_bump_upd", "grant_scopes", `CREATE TRIGGER grant_scopes_bump_upd AFTER UPDATE ON grant_scopes REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("grant_scopes_exclusion_guard", "grant_scopes", `CREATE CONSTRAINT TRIGGER grant_scopes_exclusion_guard AFTER INSERT OR UPDATE ON grant_scopes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN ((new.mode = 'exclude'::text)) EXECUTE FUNCTION trg_exclusion_needs_include()`),
		storm.Trigger("grant_scopes_self_guard", "grant_scopes", `CREATE CONSTRAINT TRIGGER grant_scopes_self_guard AFTER INSERT OR UPDATE ON grant_scopes DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION trg_self_scope_guard()`),
		storm.Trigger("grants_bump_del", "grants", `CREATE TRIGGER grants_bump_del AFTER DELETE ON grants REFERENCING OLD TABLE AS oldtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("grants_bump_ins", "grants", `CREATE TRIGGER grants_bump_ins AFTER INSERT ON grants REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("grants_bump_upd", "grants", `CREATE TRIGGER grants_bump_upd AFTER UPDATE ON grants REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("grants_realm_guard", "grants", `CREATE CONSTRAINT TRIGGER grants_realm_guard AFTER INSERT OR UPDATE OF role_id, identity_id ON grants DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION trg_grant_realm_guard()`),
		storm.Trigger("grants_role_live", "grants", `CREATE CONSTRAINT TRIGGER grants_role_live AFTER INSERT OR UPDATE OF role_id ON grants DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION trg_grant_role_live()`),
		storm.Trigger("bump_identities_del", "identities", `CREATE TRIGGER bump_identities_del AFTER DELETE ON identities REFERENCING OLD TABLE AS oldtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_identities_stmt()`),
		storm.Trigger("bump_identities_ins", "identities", `CREATE TRIGGER bump_identities_ins AFTER INSERT ON identities REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_identities_stmt()`),
		storm.Trigger("bump_identities_state", "identities", `CREATE TRIGGER bump_identities_state AFTER UPDATE ON identities REFERENCING OLD TABLE AS oldtab NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_identities_stmt()`),
		storm.Trigger("membership_entry_scopes_exclusion_guard", "membership_entry_scopes", `CREATE CONSTRAINT TRIGGER membership_entry_scopes_exclusion_guard AFTER INSERT OR UPDATE ON membership_entry_scopes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN ((new.mode = 'exclude'::text)) EXECUTE FUNCTION trg_entry_exclusion_needs_include()`),
		storm.Trigger("permissions_bump_del", "permissions", `CREATE TRIGGER permissions_bump_del AFTER DELETE ON permissions REFERENCING OLD TABLE AS oldtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("permissions_bump_ins", "permissions", `CREATE TRIGGER permissions_bump_ins AFTER INSERT ON permissions REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("permissions_bump_upd", "permissions", `CREATE TRIGGER permissions_bump_upd AFTER UPDATE ON permissions REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("realms_bump_ins", "realms", `CREATE TRIGGER realms_bump_ins AFTER INSERT ON realms REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("realms_bump_upd", "realms", `CREATE TRIGGER realms_bump_upd AFTER UPDATE ON realms REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("bump_rpe_del", "role_permissions_effective", `CREATE TRIGGER bump_rpe_del AFTER DELETE ON role_permissions_effective REFERENCING OLD TABLE AS oldtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_via_role()`),
		storm.Trigger("bump_rpe_ins", "role_permissions_effective", `CREATE TRIGGER bump_rpe_ins AFTER INSERT ON role_permissions_effective REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_via_role()`),
		storm.Trigger("bump_rpe_upd", "role_permissions_effective", `CREATE TRIGGER bump_rpe_upd AFTER UPDATE ON role_permissions_effective REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_via_role()`),
		storm.Trigger("roles_bump_del", "roles", `CREATE TRIGGER roles_bump_del AFTER DELETE ON roles REFERENCING OLD TABLE AS oldtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("roles_bump_ins", "roles", `CREATE TRIGGER roles_bump_ins AFTER INSERT ON roles REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("roles_bump_upd", "roles", `CREATE TRIGGER roles_bump_upd AFTER UPDATE ON roles REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("route_policies_bump_del", "route_policies", `CREATE TRIGGER route_policies_bump_del AFTER DELETE ON route_policies REFERENCING OLD TABLE AS oldtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("route_policies_bump_ins", "route_policies", `CREATE TRIGGER route_policies_bump_ins AFTER INSERT ON route_policies REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("route_policies_bump_upd", "route_policies", `CREATE TRIGGER route_policies_bump_upd AFTER UPDATE ON route_policies REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("bump_scope_axes", "scope_axes", `CREATE TRIGGER bump_scope_axes AFTER INSERT OR DELETE OR UPDATE ON scope_axes FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_all_tenants()`),
		storm.Trigger("scope_nodes_bump_del", "scope_nodes", `CREATE TRIGGER scope_nodes_bump_del AFTER DELETE ON scope_nodes REFERENCING OLD TABLE AS oldtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("scope_nodes_bump_ins", "scope_nodes", `CREATE TRIGGER scope_nodes_bump_ins AFTER INSERT ON scope_nodes REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("scope_nodes_bump_upd", "scope_nodes", `CREATE TRIGGER scope_nodes_bump_upd AFTER UPDATE ON scope_nodes REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()`),
		storm.Trigger("scope_nodes_type_guard", "scope_nodes", `CREATE CONSTRAINT TRIGGER scope_nodes_type_guard AFTER INSERT OR UPDATE OF parent_id, node_type ON scope_nodes DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION trg_scope_node_type_guard()`),
		storm.Trigger("bump_sessions_revoked", "sessions", `CREATE TRIGGER bump_sessions_revoked AFTER UPDATE ON sessions REFERENCING OLD TABLE AS oldtab NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_sessions_stmt()`),
		storm.Trigger("seed_auth_pages", "tenants", `CREATE TRIGGER seed_auth_pages AFTER INSERT ON tenants REFERENCING NEW TABLE AS newtab FOR EACH STATEMENT EXECUTE FUNCTION trg_seed_auth_pages()`),
	}
}
