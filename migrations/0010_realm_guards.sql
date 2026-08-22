-- ============================================================================
-- 0010_realm_guards.sql — schema-enforced guards for delegated administration
--
-- Once partners administer their own users, grant administration is no longer
-- performed only by trusted IT staff. Two escalation paths must be closed in
-- the DATABASE, not in an admin-UI validation:
--
--   1. Attaching an employee-only role to a public self-registered account.
--   2. A delegated admin conferring a role they were never authorised to confer.
--
-- Array containment cannot be expressed as a foreign key, so these are
-- constraint triggers. They still run inside the writer's transaction and
-- cannot be bypassed by a script that skips the application layer.
-- ============================================================================

CREATE OR REPLACE FUNCTION trg_grant_realm_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
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
$fn$;

CREATE CONSTRAINT TRIGGER grants_realm_guard
    AFTER INSERT OR UPDATE OF role_id, identity_id ON grants
    DEFERRABLE INITIALLY IMMEDIATE
    FOR EACH ROW EXECUTE FUNCTION trg_grant_realm_guard();

-- ---------------------------------------------------------------------------
-- Self-scoped grants are meaningless with scope constraints attached: "only
-- your own record" is orthogonal to "anywhere under Jakarta". Allowing both
-- invites a grant nobody can reason about.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION trg_self_scope_guard()
RETURNS trigger LANGUAGE plpgsql AS $fn$
BEGIN
    IF EXISTS (SELECT 1 FROM grants g
                WHERE g.id = NEW.grant_id AND g.self_scoped) THEN
        RAISE EXCEPTION 'self-scoped grant % may not carry axis constraints',
              NEW.grant_id USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$fn$;

CREATE CONSTRAINT TRIGGER grant_scopes_self_guard
    AFTER INSERT OR UPDATE ON grant_scopes
    DEFERRABLE INITIALLY IMMEDIATE
    FOR EACH ROW EXECUTE FUNCTION trg_self_scope_guard();
