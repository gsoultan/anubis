-- ============================================================================
-- 0015_memberships.sql — memberships: named bundles of (role, place) entries
--
-- The Okta-group / AD-group pattern. A membership DEFINES access as template
-- entries; assigning a person MATERIALISES real grants rows carrying
-- provenance (via_membership_id, via_entry_id). Design decisions:
--
--   LINK, NOT COPY   Derived grants stay tied to their entry. Unassign revokes
--                    them; membership_resync() reconciles after edits (add
--                    missing, revoke orphaned) — same belt-and-braces pattern
--                    as role_permissions_effective.
--   MATERIALISED     authorize() is untouched: derived grants are ordinary
--                    rows on the same hot index. Flatten at write, probe at
--                    read — the project's standing rule.
--   GUARDS FOR FREE  Fan-out INSERTs into grants fire the SAME realm-guard
--                    trigger as direct grants: an internal-only membership
--                    assigned to a public applicant aborts the whole
--                    transaction. Memberships cannot be an escalation path.
--   FLAT             No membership-in-membership. Nesting is where group
--                    audits die.
-- ============================================================================

CREATE TABLE memberships (
    created_at  timestamptz NOT NULL DEFAULT now(),
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        text NOT NULL,
    description text NOT NULL DEFAULT '',
    UNIQUE (tenant_id, name),
    UNIQUE (id, tenant_id)
);

-- Template entries: what the membership confers. Shape mirrors grants.
CREATE TABLE membership_entries (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    membership_id uuid NOT NULL,
    tenant_id     uuid NOT NULL,
    role_id       uuid NOT NULL,
    FOREIGN KEY (membership_id, tenant_id) REFERENCES memberships(id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (role_id, tenant_id)       REFERENCES roles(id, tenant_id) ON DELETE CASCADE,
    UNIQUE (id, tenant_id)
);

CREATE TABLE membership_entry_scopes (
    entry_id      uuid NOT NULL,
    tenant_id     uuid NOT NULL,
    scope_node_id uuid NOT NULL,
    inherit       boolean NOT NULL DEFAULT true,
    axis_code     text NOT NULL REFERENCES scope_axes(code) ON DELETE RESTRICT,
    PRIMARY KEY (entry_id, axis_code, scope_node_id),
    FOREIGN KEY (entry_id, tenant_id) REFERENCES membership_entries(id, tenant_id) ON DELETE CASCADE,
    -- same cross-tenant/cross-axis impossibility as grant_scopes
    FOREIGN KEY (scope_node_id, tenant_id, axis_code)
        REFERENCES scope_nodes (id, tenant_id, axis_code) ON DELETE RESTRICT
);

CREATE TABLE membership_members (
    assigned_at   timestamptz NOT NULL DEFAULT now(),
    membership_id uuid NOT NULL,
    identity_id   uuid NOT NULL,
    tenant_id     uuid NOT NULL,
    assigned_by   uuid NOT NULL,
    PRIMARY KEY (membership_id, identity_id),
    FOREIGN KEY (membership_id, tenant_id) REFERENCES memberships(id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (identity_id, tenant_id)   REFERENCES identities(id, tenant_id) ON DELETE CASCADE
);

-- Provenance on derived grants. NULL = granted directly.
ALTER TABLE grants
    ADD COLUMN via_membership_id uuid REFERENCES memberships(id) ON DELETE RESTRICT,
    ADD COLUMN via_entry_id      uuid REFERENCES membership_entries(id) ON DELETE RESTRICT;
CREATE INDEX grants_via_membership ON grants (via_membership_id, identity_id)
    WHERE via_membership_id IS NOT NULL AND revoked_at IS NULL;

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

        INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id, inherit)
        SELECT v_grant, tenant_id, axis_code, scope_node_id, inherit
          FROM membership_entry_scopes WHERE entry_id = e.id;
        v_n := v_n + 1;
    END LOOP;
    RETURN v_n;
END;
$fn$;

CREATE OR REPLACE FUNCTION membership_unassign(p_identity uuid, p_membership uuid)
RETURNS int LANGUAGE plpgsql AS $fn$
DECLARE v_n int;
BEGIN
    UPDATE grants SET revoked_at = now()
     WHERE identity_id = p_identity AND via_membership_id = p_membership
       AND revoked_at IS NULL;
    GET DIAGNOSTICS v_n = ROW_COUNT;
    DELETE FROM membership_members
     WHERE membership_id = p_membership AND identity_id = p_identity;
    RETURN v_n;
END;
$fn$;

-- After editing a membership's entries: every member gains the new entries and
-- loses grants whose entry no longer exists. Run in the editing transaction.
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
            INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id, inherit)
            SELECT v_grant, tenant_id, axis_code, scope_node_id, inherit
              FROM membership_entry_scopes WHERE entry_id = e.id;
            v_n := v_n + 1;
        END LOOP;
    END LOOP;
    RETURN v_n;
END;
$fn$;
