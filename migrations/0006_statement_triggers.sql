-- ============================================================================
-- 0006_statement_triggers.sql
--
-- FIX: the catalog-invalidation triggers in 0005 were FOR EACH ROW.
-- A bulk sync of a 20,000-node customer tree would therefore fire 20,000
-- separate UPDATEs against a single catalog_version row -- self-inflicted
-- lock contention on one hot tuple -- plus 20,000 pg_notify() calls, which
-- go through a fixed-size queue that fails the whole transaction when full.
--
-- Statement-level triggers with transition tables collapse that to ONE bump
-- per statement regardless of row count. Invalidation semantics are
-- unchanged: readers only care that the version moved, not how far.
-- ============================================================================

DROP TRIGGER IF EXISTS bump_scope_nodes    ON scope_nodes;
DROP TRIGGER IF EXISTS bump_grants         ON grants;
DROP TRIGGER IF EXISTS bump_grant_scopes   ON grant_scopes;
DROP TRIGGER IF EXISTS bump_roles          ON roles;
DROP TRIGGER IF EXISTS bump_permissions    ON permissions;
DROP TRIGGER IF EXISTS bump_route_policies ON route_policies;

CREATE OR REPLACE FUNCTION trg_bump_catalog_stmt()
RETURNS trigger LANGUAGE plpgsql AS $fn$
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
$fn$;

-- Transition tables are per-operation: INSERT exposes NEW TABLE, DELETE
-- exposes OLD TABLE, so each table needs three triggers.
DO $do$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['scope_nodes','grants','grant_scopes',
                             'roles','permissions','route_policies']
    LOOP
        EXECUTE format($f$
            CREATE TRIGGER %1$s_bump_ins AFTER INSERT ON %1$s
            REFERENCING NEW TABLE AS newtab
            FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()$f$, t);
        EXECUTE format($f$
            CREATE TRIGGER %1$s_bump_upd AFTER UPDATE ON %1$s
            REFERENCING NEW TABLE AS newtab
            FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()$f$, t);
        EXECUTE format($f$
            CREATE TRIGGER %1$s_bump_del AFTER DELETE ON %1$s
            REFERENCING OLD TABLE AS oldtab
            FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt()$f$, t);
    END LOOP;
END;
$do$;
