-- ============================================================================
-- 0046_scope_exclusions.sql — "everywhere under Jakarta except the Surabaya
-- branch", said once instead of enumerated
--
-- Until now every grant_scopes row was an include, and the only way to write
-- "all of Jakarta except one branch" was to enumerate Jakarta's other
-- children — a list that is correct on the day it is written and silently
-- wrong the next time somebody opens a branch. The carve-out is the thing the
-- operator means; the enumeration is a snapshot of it that nothing maintains.
--
-- ADR-0004 rejects deny GRANTS, and this is deliberately not one. The
-- distinction is the whole reason this is shippable:
--
--   * An exclusion narrows THE GRANT IT SITS ON. It is read as part of that
--     grant's sentence, and a grant still reads on its own.
--   * It does NOT suppress another grant. If a second grant covers Surabaya,
--     access there survives — that grant says so, and nothing hidden
--     elsewhere overrides it.
--
-- That keeps the property ADR-0004 was protecting: to know why someone has
-- access you read the grants that give it, and to know why they do not you
-- read the same list. There is no global precedence order to learn, because
-- there is no cross-grant deny.
--
-- THE RULE, whole:  include says where; exclude carves back out of it.
--
--   satisfied(grant, axis) = (some include covers the target)
--                        AND (no exclude covers the target)
--
-- An exclusion with no include on the same axis would mean "anywhere except
-- here", which is a blanket allow wearing a carve-out's clothes — and on a
-- strict axis it would defeat the one thing strict exists to force, a grant
-- naming where it applies. So it is refused at write time by a constraint
-- trigger, and if one ever existed anyway the aggregate above evaluates it to
-- false: fail-closed, not fail-open. The "everything except" case is still
-- expressible, by including the axis root and excluding the exception, which
-- is also how it should read on screen.
--
-- Nothing existing changes behaviour. Every row that exists is an include
-- (the column default), and with no exclude rows in a group the added
-- conjunct is NOT bool_or(false) = true, leaving satisfied bit-identical to
-- 0013. Measured, not assumed: 2,000 recorded decisions replayed either side
-- of this migration → 0 changed verdicts.
--
-- The three surfaces that re-derive these semantics are all rewritten here or
-- alongside: authorize(), authorize_explain(), and AuthorizeStrictSim in
-- authzrquery. The fourth, the gate's in-memory evaluator, is Go and is held
-- to it by test/integration/snapshot_parity_test.go.
-- ============================================================================

SET LOCAL lock_timeout = '5s';

-- A non-volatile default, so this is a catalog update and not a rewrite of
-- 270k rows. Text with a CHECK rather than a boolean: `mode = 'exclude'` is
-- readable in a WHERE clause a year from now, `NOT included` is not, and the
-- enum leaves room for a third mode without another rewrite.
ALTER TABLE grant_scopes
    ADD COLUMN mode text NOT NULL DEFAULT 'include'
                         CHECK (mode IN ('include', 'exclude'));

ALTER TABLE membership_entry_scopes
    ADD COLUMN mode text NOT NULL DEFAULT 'include'
                         CHECK (mode IN ('include', 'exclude'));

COMMENT ON COLUMN grant_scopes.mode IS
    'include = the grant applies here; exclude = carved out of this grant''s includes on the same axis.';
COMMENT ON COLUMN membership_entry_scopes.mode IS
    'Copied verbatim onto every grant this entry materialises. See grant_scopes.mode.';

-- ---------------------------------------------------------------------------
-- An exclusion with nothing to exclude from
--
-- DEFERRED, not IMMEDIATE like the two guards in 0010: the include and the
-- exclude arrive as separate INSERTs in one transaction and the writer does
-- not promise an order, so the question can only be answered at commit.
--
-- Deliberately not armed on DELETE. grant_scopes rows are never deleted
-- individually — the API only inserts, and a grant is revoked rather than
-- removed — so the only DELETE is the cascade when a grant row goes, during
-- which "the include is missing" is true of every exclude row and means
-- nothing. A DELETE arm would turn the cascade into an error.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION trg_exclusion_needs_include()
RETURNS trigger LANGUAGE plpgsql AS $fn$
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
$fn$;

CREATE CONSTRAINT TRIGGER grant_scopes_exclusion_guard
    AFTER INSERT OR UPDATE ON grant_scopes
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW WHEN (NEW.mode = 'exclude')
    EXECUTE FUNCTION trg_exclusion_needs_include();

CREATE OR REPLACE FUNCTION trg_entry_exclusion_needs_include()
RETURNS trigger LANGUAGE plpgsql AS $fn$
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
$fn$;

CREATE CONSTRAINT TRIGGER membership_entry_scopes_exclusion_guard
    AFTER INSERT OR UPDATE ON membership_entry_scopes
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW WHEN (NEW.mode = 'exclude')
    EXECUTE FUNCTION trg_entry_exclusion_needs_include();

-- ---------------------------------------------------------------------------
-- authorize() — 0013's shape, with the veto folded into its aggregate
--
-- The correlated closure probe 0007 measured and 0013 kept is untouched, and
-- still runs exactly once per grant_scopes row of a candidate grant. What
-- changes is what the rows vote into.
--
-- The obvious spelling of the rule is two aggregates —
--
--     bool_or(mode='include' AND covers) AND NOT bool_or(mode='exclude' AND covers)
--
-- which reads like the sentence and costs 9%. Measured interleaved,
-- best-of-15, 2,000 decisions against the 270k-scope dev database:
--
--     0013 as it stands          112.0 ms best / 113.0 median
--     two bool_or aggregates     122.1 ms       / 123.4          +9.2%
--     one min() aggregate        113.4 ms       / 114.4          +1.2%
--
-- 5 µs a decision is not fatal against a 2 ms budget, but it is eight times
-- the price of the same semantics, so the hot path takes the cheap one and
-- pays for it in a comment. (authorize_explain below keeps the two-aggregate
-- form: it is an operator tool, not a request path, and it has to report the
-- two halves separately anyway.)
--
-- The encoding is a severity order — a veto outranks an allow, an allow
-- outranks silence — so min() picks the strongest opinion the axis holds:
--
--     0  an exclude covers the target   → vetoed, whatever else is here
--     1  an include covers the target   → allowed, if nothing vetoes
--     2  this row does not cover it     → says nothing
--
-- min = 1 is therefore exactly "something included the target and nothing
-- excluded it". min = 2 (nothing covered it) and min = 0 (something vetoed)
-- both deny, which is also what an exclude-only axis evaluates to if one ever
-- slips past the guard above: fail-closed, never fail-open.
-- ---------------------------------------------------------------------------
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
$fn$;

-- ---------------------------------------------------------------------------
-- authorize_explain() — 0020's decomposition, told the difference between
-- "your scope does not match" and "your scope was carved out"
--
-- Those two denials look identical in the old output and need opposite
-- actions: one is a grant that was never meant to reach here, the other is a
-- grant that WAS and has an exception on it. 'included' and 'excluded' are
-- reported per axis, every node now carries its mode, and a denial that an
-- exclusion actually decided gets its own reason.
--
-- PARITY RULE (0020) is unchanged: 'allow' still comes from authorize().
-- ---------------------------------------------------------------------------
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
$fn$;

-- ---------------------------------------------------------------------------
-- The membership fan-out (0015) copies scopes onto every grant it
-- materialises. It named its columns one by one, so an entry's exclusions
-- would have been dropped on the way to the grant — an access widening, made
-- of silence, that no error would have reported. Both functions are restated
-- here for the one added column.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION membership_assign(
    p_identity uuid, p_membership uuid, p_by uuid
) RETURNS int LANGUAGE plpgsql AS $fn$
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
$fn$;

CREATE OR REPLACE FUNCTION membership_resync(p_membership uuid)
RETURNS int LANGUAGE plpgsql AS $fn$
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
$fn$;
