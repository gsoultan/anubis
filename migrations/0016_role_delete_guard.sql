-- ============================================================================
-- 0016_role_delete_guard.sql — deleting a role must be loud, not polite
--
-- grants.role_id and membership_entries.role_id were ON DELETE CASCADE: drop a
-- role and every grant referencing it — the record of WHO HELD WHAT — vanishes
-- silently, and memberships lose entries without anyone consenting. For an
-- audit-bearing system that is exactly wrong. RESTRICT inverts it: a role with
-- live consequences cannot be deleted; revoke the access and unbundle it
-- first, each step leaving its own trail.
-- ============================================================================

ALTER TABLE grants
    DROP CONSTRAINT grants_role_id_tenant_id_fkey,
    ADD CONSTRAINT grants_role_id_tenant_id_fkey
        FOREIGN KEY (role_id, tenant_id)
        REFERENCES roles(id, tenant_id) ON DELETE RESTRICT;

ALTER TABLE membership_entries
    DROP CONSTRAINT membership_entries_role_id_tenant_id_fkey,
    ADD CONSTRAINT membership_entries_role_id_tenant_id_fkey
        FOREIGN KEY (role_id, tenant_id)
        REFERENCES roles(id, tenant_id) ON DELETE RESTRICT;
