-- ---------------------------------------------------------------------------
-- A role can be retired without taking anybody's access away.
--
-- Permissions have had this since 0004: a manifest that stops naming one
-- stamps deprecated_at, and the permission keeps working for every grant that
-- already refers to it. Roles had no such column, so "removed from the
-- document" meant nothing at all — a role dropped from a manifest, or from a
-- synced CSV, stayed fully grantable forever. The catalog said one thing and
-- the grant screen offered another.
--
-- DEPRECATED IS NOT REVOKED. This is the whole point and it is worth being
-- exact about, because the two are one word apart and a long way apart in
-- consequence:
--
--   * authorize() does not look at this column, and must not. Every grant
--     that names a deprecated role keeps deciding exactly as it did, so
--     retiring a role at 09:00 does not lock anybody out at 09:01.
--   * What stops is NEW grants. The trigger below refuses an insert naming a
--     deprecated role, which is the same enforcement layer that already holds
--     the realm-kind guard and role_grantable (0010) — so it holds for the
--     console, the API, a bulk import and anything written next.
--
-- Retiring access is a separate, deliberate act: revoke the grants.
--
-- Only MANIFEST-OWNED roles are ever deprecated automatically (is_system).
-- A role an operator created by hand inside the same application is theirs,
-- and a document that does not mention it is not evidence they wanted it
-- gone — see DeprecateRolesExcept in the authz rquery package.
-- ---------------------------------------------------------------------------

SET LOCAL lock_timeout = '5s';

ALTER TABLE roles ADD COLUMN deprecated_at timestamptz;

COMMENT ON COLUMN roles.deprecated_at IS
    'Retired from the catalog: cannot be granted anew, keeps working for every grant that already names it. NULL = live.';

-- ---------------------------------------------------------------------------
-- The guard. A constraint trigger rather than a check in application code:
-- the rules about what may be granted already live here (0010), and a rule
-- that lives in one code path is a rule the next code path forgets.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION trg_grant_role_live()
RETURNS trigger LANGUAGE plpgsql AS $fn$
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
$fn$;

-- Fires only when a grant starts naming a role, so the rows that already do
-- are never re-examined: deprecating a role must not make an existing grant
-- unwritable, or the next unrelated UPDATE on it would fail.
CREATE CONSTRAINT TRIGGER grants_role_live
    AFTER INSERT OR UPDATE OF role_id ON grants
    DEFERRABLE INITIALLY IMMEDIATE
    FOR EACH ROW EXECUTE FUNCTION trg_grant_role_live();
