-- ============================================================================
-- 0025 — The platform control plane (ADR-0011).
--
-- Anubis had exactly one kind of authority: a grant, held by an identity,
-- inside a tenant. That serves the people who USE an installation. It has
-- never served the people who RUN one — whoever creates tenants, and hands an
-- administrator responsibility for a tenant they are not a member of.
--
-- The tempting fix is to let a grant point across tenants. This migration does
-- not do that, and must never be amended to. 0008 says why:
--
--   Every grant/scope FK in this schema is deliberately composite on tenant_id
--   precisely to make cross-tenant references impossible to insert -- the
--   single strongest control in the design.
--
-- So operator authority is a SEPARATE mechanism that never borrows the grant
-- tables. Nothing below touches grants, grant_scopes, memberships or scope
-- nodes; the data plane is exactly as it was.
-- ============================================================================

-- ---------------------------------------------------------------------------
-- One tenant is the platform tenant, and operators are ordinary identities
-- inside it. Reusing identities buys password policy, MFA, sessions, PASETO,
-- refresh-token theft detection, retention and audit unchanged; a second table
-- of humans would have meant a second copy of every one of those controls, and
-- the second copy is the one that rots.
-- ---------------------------------------------------------------------------
ALTER TABLE tenants ADD COLUMN is_platform boolean NOT NULL DEFAULT false;

-- At most one. A second platform tenant would mean two disjoint sets of
-- operators each believing they run the installation.
CREATE UNIQUE INDEX tenants_one_platform ON tenants (is_platform) WHERE is_platform;

-- Referenced by platform_assignments below to pin an operator's tenant.
ALTER TABLE tenants ADD CONSTRAINT tenants_platform_key UNIQUE (id, is_platform);

-- ---------------------------------------------------------------------------
-- platform_assignments — the whole of the control plane's authority.
--
-- tenant_id IS NULL means every tenant: the installation owner. Any other row
-- names one tenant, which is an administrator made responsible for exactly
-- that tenant.
-- ---------------------------------------------------------------------------
CREATE TABLE platform_assignments (
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),

    operator_id        uuid NOT NULL,
    -- Carried so the composite keys below can do their work. It is the
    -- operator's OWN tenant, which the constraints force to be the platform
    -- tenant -- never the tenant being administered.
    operator_tenant_id uuid NOT NULL,
    -- Pinned true, so the second foreign key can only resolve against a tenant
    -- whose is_platform is true. This is the same declarative trick the rest of
    -- the schema uses for cross-tenant impossibility: an operator outside the
    -- platform tenant is not rejected by application code, it is unstorable.
    operator_platform  boolean NOT NULL DEFAULT true CHECK (operator_platform),

    tenant_id          uuid REFERENCES tenants(id) ON DELETE CASCADE,

    -- What the operator may do in the tenants this assignment covers.
    --   support : read configuration and administer people
    --   admin   : everything an in-tenant administrator may do
    --   owner   : admin, plus assigning other operators to that tenant
    role               text NOT NULL CHECK (role IN ('support','admin','owner')),

    granted_by         uuid REFERENCES identities(id) ON DELETE SET NULL,
    reason             text NOT NULL DEFAULT '',
    valid_until        timestamptz,
    revoked_at         timestamptz,

    FOREIGN KEY (operator_id, operator_tenant_id)
        REFERENCES identities(id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (operator_tenant_id, operator_platform)
        REFERENCES tenants(id, is_platform)
);

-- One live assignment per operator per tenant. Two rows granting the same
-- operator different roles in one tenant has no defensible meaning, and
-- whichever the reader picked would be the wrong one half the time.
CREATE UNIQUE INDEX platform_assignments_live
    ON platform_assignments (operator_id, tenant_id)
    WHERE revoked_at IS NULL AND tenant_id IS NOT NULL;

-- NULL tenant_id is "every tenant", and UNIQUE treats NULLs as distinct, so
-- the installation-owner row needs its own index or it can be inserted twice.
CREATE UNIQUE INDEX platform_assignments_live_global
    ON platform_assignments (operator_id)
    WHERE revoked_at IS NULL AND tenant_id IS NULL;

-- The lookup on the guard's operator path: every admin call an operator makes
-- resolves its assignment by (operator, tenant).
CREATE INDEX platform_assignments_lookup
    ON platform_assignments (operator_id, tenant_id, revoked_at);

COMMENT ON TABLE platform_assignments IS
    'Control-plane authority (ADR-0011). Never a substitute for a grant: this '
    'says which tenants an operator may administer, not what a member may do.';
