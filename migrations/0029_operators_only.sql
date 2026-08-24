-- ============================================================================
-- 0029 — Administration belongs to platform users. Entirely.
--
-- Until now every tenant carried a copy of Anubis's own machinery: an `anubis`
-- application owning the anubis:* permission catalog, a `console` application
-- from when the admin console signed in as a tenant identity, and an
-- `anubis.admin` role so a TENANT'S OWN person could administer their tenant
-- ("delegated administration", ADR-0011 first draft).
--
-- The owner rejected that model, and the objection is structural: it mixes
-- the two populations. A tenant's roles and permissions exist for the
-- tenant's PEOPLE and the tenant's APPLICATIONS; who may administer a tenant
-- is decided by platform_assignments and nothing else. Keeping anubis:* rows
-- in tenant catalogs meant a tenant grant could confer administration — the
-- exact mixing the two-plane split (0026) exists to prevent.
--
-- So the machinery goes. Operator authority survives untouched: the operator
-- role allow-lists compare permission STRINGS in Go and never needed these
-- rows — which is the whole point of the control plane.
-- ============================================================================

-- Everything hanging off Anubis's own applications, leaves first. Grants are
-- deleted explicitly rather than trusted to cascade: a grant of anubis.admin
-- is precisely the tenant-side administrator being removed, and negative.sql
-- proves deleting a held role does not cascade quietly.
-- Session-scoped, not ON COMMIT DROP: the shell paths (db.sh, rebuild.sh)
-- apply this file in autocommit, where ON COMMIT DROP would vanish between
-- statements. Dropped explicitly below instead.
CREATE TEMPORARY TABLE _sys_apps AS
    SELECT id FROM applications WHERE is_system;
CREATE TEMPORARY TABLE _sys_roles AS
    SELECT id FROM roles WHERE application_id IN (SELECT id FROM _sys_apps);

DELETE FROM grants
 WHERE role_id IN (SELECT id FROM _sys_roles);
DELETE FROM membership_entries
 WHERE role_id IN (SELECT id FROM _sys_roles);
DELETE FROM roles
 WHERE id IN (SELECT id FROM _sys_roles);
DELETE FROM permissions
 WHERE application_id IN (SELECT id FROM _sys_apps);
DELETE FROM applications
 WHERE id IN (SELECT id FROM _sys_apps);

-- With no system applications possible, the marker (0028) is dead weight.
-- Nothing creates one anymore; a column that guards against a state that
-- cannot occur is a column somebody will eventually misread as load-bearing.
DROP TABLE _sys_roles;
DROP TABLE _sys_apps;

DROP INDEX IF EXISTS applications_tenant_visible;
ALTER TABLE applications DROP COLUMN IF EXISTS is_system;

COMMENT ON TABLE roles IS
    'The tenant''s own roles: what its people can be given. Administration '
    'of a tenant is never here — that is platform_assignments (ADR-0011).';
