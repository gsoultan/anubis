-- ============================================================================
-- 0037 — the public schema stops being public.
--
-- 0023 gave the runtime least privilege and said why: "makes that refusal
-- enforceable rather than aspirational". It granted USAGE ON SCHEMA public
-- explicitly to anubis_app and anubis_readonly — and left PUBLIC's implicit
-- grant in place, so those explicit grants decided nothing. Any role in the
-- database reached the schema through PUBLIC regardless.
--
-- Postgres 15 already revokes CREATE from PUBLIC by default, so the schema
-- could not be modified. What remained was reachability: a role added later
-- for a reporting tool, a misconfigured integration, a compromised
-- least-privilege login — each could read whatever table grants allowed,
-- without anyone having granted it the schema.
--
-- Measured, on a database with this applied. A role holding anubis_app reads
-- normally. A role holding nothing gets:
--
--     SELECT 1 FROM tenants  ->  ERROR: relation "tenants" does not exist
--
-- Be precise about what this is not: pg_catalog stays world-readable, as it
-- does in every Postgres, so that role still counts 53 table NAMES through
-- pg_tables. This closes access to the data, not knowledge that the tables
-- exist. Anyone claiming otherwise has not tried it.
--
-- Found by diffing a freshly migrated database against the development one,
-- which had this revoke applied by hand and had been running with it. That is
-- the evidence it is safe: the roles that need USAGE hold it explicitly, and
-- have done since 0023.
--
-- If a later role needs the schema, grant it USAGE by name. That is the point.
-- ============================================================================

REVOKE USAGE ON SCHEMA public FROM PUBLIC;

-- Belt and braces: re-assert the grants 0023 made, so this migration cannot
-- lock out the runtime on a database where 0023 ran before those roles were
-- renamed or recreated.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anubis_app') THEN
        GRANT USAGE ON SCHEMA public TO anubis_app;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anubis_readonly') THEN
        GRANT USAGE ON SCHEMA public TO anubis_readonly;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anubis_owner') THEN
        GRANT USAGE, CREATE ON SCHEMA public TO anubis_owner;
    END IF;
END $$;
