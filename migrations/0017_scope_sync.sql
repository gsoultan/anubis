-- ============================================================================
-- 0017_scope_sync.sql — synchronise structures from source systems
--
-- Structures usually LIVE somewhere else — the ERP owns product lines, the CRM
-- owns customers, HR owns the org chart. A sync source per structure declares
-- where truth lives (http | db_query | db_table); the app tier fetches rows
-- and hands them to scope_sync_apply(), which reconciles transactionally:
--
--   keyed on external_ref   the idempotency key scope_nodes has carried since
--                           0003 — same ref = same node, forever
--   manual nodes untouched  sync archives only nodes carrying a ref; nodes
--                           created by hand in the console are never its to
--                           remove
--   archive, never delete   grants reference nodes (ADR-0003) — a vanished
--                           source row archives the node, existing access
--                           keeps working, pickers stop offering it
--   level rules still bite  inserts/moves run through scope_add_node /
--                           scope_move_node, so the 0014 type guard rejects a
--                           feed that tries an illegal placement — per row,
--                           captured as errors, without aborting the run
--   dry run first-class     p_dry computes the same report with zero DML
--
-- Feed contract: rows ordered parents-before-children ({ref, parent_ref,
-- name, node_type?}); parent_ref NULL attaches under the axis root.
-- ============================================================================

CREATE TABLE scope_sync_sources (
    created_at  timestamptz NOT NULL DEFAULT now(),
    last_run_at timestamptz,
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    axis_code   text NOT NULL REFERENCES scope_axes(code) ON DELETE CASCADE,
    kind        text NOT NULL CHECK (kind IN ('http','db_query','db_table')),
    status      text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    -- http:     {"url":..., "auth_header":...}
    -- db_query: {"dsn":..., "query":...}
    -- db_table: {"dsn":..., "table":..., "columns":{"ref":..,"parent_ref":..,"name":..}}
    -- plus default_node_type for rows that do not carry one
    config      jsonb NOT NULL,
    UNIQUE (tenant_id, axis_code)          -- one source of truth per structure
);

CREATE TABLE scope_sync_runs (
    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    source_id   uuid NOT NULL REFERENCES scope_sync_sources(id) ON DELETE CASCADE,
    dry         boolean NOT NULL DEFAULT false,
    status      text NOT NULL DEFAULT 'running'
                CHECK (status IN ('running','ok','failed','dry_run')),
    report      jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX scope_sync_runs_src ON scope_sync_runs (source_id, started_at DESC);

CREATE OR REPLACE FUNCTION scope_sync_apply(
    p_source uuid, p_rows jsonb, p_dry boolean DEFAULT false
) RETURNS jsonb LANGUAGE plpgsql AS $fn$
DECLARE
    src record; root_id uuid; e jsonb;
    v_ref text; v_pref text; v_name text; v_type text;
    v_node record; v_parent uuid;
    n_add int := 0; n_ren int := 0; n_mov int := 0; n_arc int := 0; n_same int := 0;
    errs jsonb := '[]'; rep jsonb; run_id uuid;
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
                    RAISE EXCEPTION 'parent ref "%" not found (feed must list parents first)', v_pref;
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
$fn$;
