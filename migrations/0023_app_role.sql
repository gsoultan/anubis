-- ============================================================================
-- 0023_app_role.sql — least privilege for the runtime role
--
-- The application must not be able to change the schema. ADR-0005 §7 refused
-- to partition scope_nodes by axis precisely because it would require giving
-- the app CREATE rights: "you have traded a deploy for a privilege-escalation
-- path". This migration makes that refusal enforceable rather than aspirational.
--
-- Two roles:
--   anubis_owner — owns the schema, runs migrations. Used by `anubisd migrate`
--                  and by a human at the psql prompt. Never by the server.
--   anubis_app   — the runtime. SELECT/INSERT/UPDATE/DELETE on tables,
--                  EXECUTE on the decision functions, and nothing else:
--                  no CREATE, no DROP, no ALTER, no TRUNCATE.
--
-- Deliberately NOT revoked from anubis_app:
--   * DELETE on one_time_tokens — single-use consumption IS a delete
--   * DELETE on pii_keys — crypto-shredding IS a delete (that is the point)
--   * UPDATE on audit_log is NOT granted: the chain is append-only, and the
--     hash chain exists to detect exactly the tampering this forbids.
--
-- Idempotent and safe on a database where the roles already exist; CREATE ROLE
-- is guarded because a managed Postgres may have provisioned them already.
-- ============================================================================

DO $do$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anubis_app') THEN
        -- NOLOGIN by default: the deployment grants LOGIN and a password, so
        -- no credential is ever written into a migration file.
        CREATE ROLE anubis_app NOLOGIN;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anubis_readonly') THEN
        CREATE ROLE anubis_readonly NOLOGIN;
    END IF;
END;
$do$;

GRANT USAGE ON SCHEMA public TO anubis_app, anubis_readonly;

-- Runtime: data only.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO anubis_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO anubis_app;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO anubis_app;

-- Append-only means append-only: no UPDATE, no DELETE on the audit chain.
REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM anubis_app;
REVOKE UPDATE, DELETE, TRUNCATE ON pii_key_tombstones FROM anubis_app;
-- Migration bookkeeping belongs to the owner.
REVOKE INSERT, UPDATE, DELETE ON schema_migrations FROM anubis_app;

-- Analysts and dashboards: reads, and never the secrets.
GRANT SELECT ON ALL TABLES IN SCHEMA public TO anubis_readonly;
REVOKE SELECT ON credentials, signing_keys, refresh_tokens, one_time_tokens,
       pii_keys FROM anubis_readonly;

-- Anything a later migration creates inherits the same shape.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO anubis_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO anubis_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT EXECUTE ON FUNCTIONS TO anubis_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT ON TABLES TO anubis_readonly;
