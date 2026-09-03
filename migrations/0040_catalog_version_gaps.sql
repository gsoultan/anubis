-- ---------------------------------------------------------------------------
-- Close the catalog-version gaps, so the gate's refresh can stop rebuilding
-- snapshots that have not changed.
--
-- Migrations 0005/0006 bump the version for scope_nodes, grants, grant_scopes,
-- roles, permissions and route_policies. The gate's snapshot ALSO reads five
-- tables that bump nothing, and they reached the gate only because
-- Manager.refreshAll reloads every tenant unconditionally every 30 s:
--
--     sessions                    a revocation
--     identities                  blocked / token_epoch / assurance
--     scope_axes                  a flip to default_effect='deny'
--     role_permissions_effective  a permission pulled from a role
--     applications                a renamed slug (routes match on it)
--
-- That unconditional reload is ~92 MB per tenant at a million scope nodes,
-- and it is what caps how many tenants one instance can carry. It cannot be
-- made conditional until every one of those five pushes an invalidation —
-- which is what this migration does.
--
-- THE FILTERS ARE THE WHOLE POINT. sessions.last_seen_at is written on every
-- authenticated request and identities.last_login_at on every login. A
-- trigger that fired on those would bump the catalog version continuously,
-- turning a snapshot reload into a per-request operation — far worse than the
-- polling it replaces. Every trigger below is narrowed to the columns that
-- actually change an authorization outcome.
--
-- LOCKING. Each CREATE TRIGGER takes a lock that conflicts with writes to its
-- table, and the runner wraps this whole file in ONE transaction — so locks
-- on sessions, identities, applications, role_permissions_effective and
-- scope_axes are all held until it commits. The work itself is metadata only
-- (milliseconds), but ACQUIRING the lock on a busy sessions table means
-- waiting for in-flight statements, and anything queued behind that DDL is
-- blocked meanwhile. lock_timeout bounds it: the migration fails and can be
-- retried in a quiet window instead of stalling writers.
-- ---------------------------------------------------------------------------

SET LOCAL lock_timeout = '5s';

-- ── sessions: revocation only ──────────────────────────────────────────────
-- STATEMENT level, like migration 0006. RevokeAllSessions is a bulk UPDATE
-- over every session an identity holds, so a FOR EACH ROW trigger would fire
-- one catalog_version upsert plus one pg_notify PER ROW, all serialised on
-- the same catalog_version row. That is exactly what 0006 moved away from.
--
-- Only the NULL -> NOT NULL transition counts, which the join below expresses:
-- re-revoking an already revoked session, touching last_seen_at (written on
-- EVERY authenticated request) and rotating a cookie hash are all silent.
-- DELETE is silent too: expiry cleanup removes sessions already outside the
-- revocation window, so the gate cannot be relying on them.
CREATE OR REPLACE FUNCTION trg_bump_catalog_sessions_stmt()
RETURNS trigger LANGUAGE plpgsql AS $fn$
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
$fn$;

CREATE TRIGGER bump_sessions_revoked
    AFTER UPDATE ON sessions
    REFERENCING OLD TABLE AS oldtab NEW TABLE AS newtab
    FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_sessions_stmt();

-- ── identities: the state gates that outrank grants ────────────────────────
-- Also statement level: identity administration is done in bulk.
--
-- On UPDATE only the columns the snapshot actually reads (SnapshotIdentities:
-- token_epoch, status, assurance_level, and the disabled/anonymised pair
-- behind `blocked`). last_login_at — written on EVERY login — and attributes
-- and pii_key_id are deliberately excluded.
--
-- INSERT and DELETE always matter: an identity absent from the snapshot is
-- denied, so a new one cannot act until the gate sees it; one still present
-- after deletion would keep being evaluated.
CREATE OR REPLACE FUNCTION trg_bump_catalog_identities_stmt()
RETURNS trigger LANGUAGE plpgsql AS $fn$
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
$fn$;

CREATE TRIGGER bump_identities_ins AFTER INSERT ON identities
    REFERENCING NEW TABLE AS newtab
    FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_identities_stmt();
CREATE TRIGGER bump_identities_del AFTER DELETE ON identities
    REFERENCING OLD TABLE AS oldtab
    FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_identities_stmt();
CREATE TRIGGER bump_identities_state AFTER UPDATE ON identities
    REFERENCING OLD TABLE AS oldtab NEW TABLE AS newtab
    FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_identities_stmt();

-- ── applications: the slug routes are matched on ───────────────────────────
CREATE OR REPLACE FUNCTION trg_bump_catalog_applications_stmt()
RETURNS trigger LANGUAGE plpgsql AS $fn$
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
$fn$;

CREATE TRIGGER bump_applications_ins AFTER INSERT ON applications
    REFERENCING NEW TABLE AS newtab
    FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_applications_stmt();
CREATE TRIGGER bump_applications_del AFTER DELETE ON applications
    REFERENCING OLD TABLE AS oldtab
    FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_applications_stmt();
CREATE TRIGGER bump_applications_slug AFTER UPDATE ON applications
    REFERENCING OLD TABLE AS oldtab NEW TABLE AS newtab
    FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_applications_stmt();

-- ── role_permissions_effective: no tenant_id, reach it through the role ────
-- Statement-level: this table is rewritten wholesale when a role hierarchy
-- changes, so a row trigger would fire thousands of times for one edit.
CREATE OR REPLACE FUNCTION trg_bump_catalog_via_role()
RETURNS trigger LANGUAGE plpgsql AS $fn$
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
$fn$;

CREATE TRIGGER bump_rpe_ins AFTER INSERT ON role_permissions_effective
    REFERENCING NEW TABLE AS newtab
    FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_via_role();
CREATE TRIGGER bump_rpe_upd AFTER UPDATE ON role_permissions_effective
    REFERENCING NEW TABLE AS newtab
    FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_via_role();
CREATE TRIGGER bump_rpe_del AFTER DELETE ON role_permissions_effective
    REFERENCING OLD TABLE AS oldtab
    FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_via_role();

-- ── scope_axes: global, so every tenant is invalidated ─────────────────────
-- An axis has no tenant_id: default_effect='deny' makes an axis STRICT for
-- everyone at once, and a grant silent on a strict axis is denied. Rare
-- enough that fanning out over tenants is the right trade.
CREATE OR REPLACE FUNCTION trg_bump_catalog_all_tenants()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE r record;
BEGIN
    FOR r IN SELECT id FROM tenants LOOP
        PERFORM bump_catalog_version(r.id);
    END LOOP;
    RETURN NULL;
END;
$fn$;

CREATE TRIGGER bump_scope_axes
    AFTER INSERT OR UPDATE OR DELETE ON scope_axes
    FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_all_tenants();
