-- ============================================================================
-- 0026 — Platform users are their own population (ADR-0011, revised).
--
-- 0025 modelled operators as identities inside a "platform tenant". That was
-- wrong. Whoever runs an installation is not a member of anything it hosts:
-- they have no realm, no grants, no scope, no retention policy, and no reason
-- to appear in any tenant's directory. Reusing `identities` made the two
-- populations look related, and every screen that touched either had to
-- remember they were not.
--
-- So: a separate table, with no path between it and identities. A tenant's
-- user cannot be promoted into an operator, an operator cannot be granted a
-- role inside a tenant, and neither can be turned into the other by editing a
-- row. That is the whole point.
--
-- Note also what falls out: platform usernames are globally unique, because
-- there is no tenant to scope them by. Signing in to the console needs a
-- username and a password and nothing else.
-- ============================================================================

CREATE TABLE platform_users (
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    id            uuid PRIMARY KEY DEFAULT uuidv7(),

    -- Same charset rule as an identity's username, so the two look alike to a
    -- person even though nothing joins them.
    username      text NOT NULL CHECK (username ~ '^[a-zA-Z0-9][a-zA-Z0-9._-]{1,62}$'),
    email         text,
    -- The credential lives here rather than in `credentials`: that table is
    -- composite on tenant_id, which an operator does not have.
    password_hash text NOT NULL,

    status        text NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active','disabled')),
    -- Bumping this invalidates the operator's live tokens, the same way
    -- identities.token_epoch does for a tenant's users.
    token_epoch   integer NOT NULL DEFAULT 0,

    last_login_at timestamptz,
    disabled_at   timestamptz
);

-- Globally unique, case-insensitively. There is no tenant to scope by, which
-- is exactly why console sign-in can ask for a username alone.
CREATE UNIQUE INDEX platform_users_username ON platform_users (lower(username));
CREATE UNIQUE INDEX platform_users_email ON platform_users (lower(email))
    WHERE email IS NOT NULL AND email <> '';

-- ---------------------------------------------------------------------------
-- platform_assignments now points at platform_users.
--
-- Dropped and recreated rather than altered: the old table's whole shape was
-- the machinery for keeping an operator inside the platform tenant, and none
-- of it means anything now. The feature is unreleased, so there is nothing to
-- preserve — this is not a pattern to copy on a table with real rows.
-- ---------------------------------------------------------------------------
DROP TABLE IF EXISTS platform_assignments;

CREATE TABLE platform_assignments (
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    id          uuid PRIMARY KEY DEFAULT uuidv7(),

    operator_id uuid NOT NULL REFERENCES platform_users(id) ON DELETE CASCADE,
    -- NULL means every tenant: the installation owner.
    tenant_id   uuid REFERENCES tenants(id) ON DELETE CASCADE,

    --   support : read configuration and administer people
    --   admin   : everything an in-tenant administrator may do
    --   owner   : admin, plus assigning other operators
    role        text NOT NULL CHECK (role IN ('support','admin','owner')),

    -- Another platform user, never an identity.
    granted_by  uuid REFERENCES platform_users(id) ON DELETE SET NULL,
    reason      text NOT NULL DEFAULT '',
    valid_until timestamptz,
    revoked_at  timestamptz
);

-- One live assignment per operator per tenant. Two rows granting the same
-- operator different roles in one tenant has no defensible meaning.
CREATE UNIQUE INDEX platform_assignments_live
    ON platform_assignments (operator_id, tenant_id)
    WHERE revoked_at IS NULL AND tenant_id IS NOT NULL;

-- NULL tenant_id is "every tenant", and UNIQUE treats NULLs as distinct, so
-- the installation-owner row needs its own index or it can be inserted twice.
CREATE UNIQUE INDEX platform_assignments_live_global
    ON platform_assignments (operator_id)
    WHERE revoked_at IS NULL AND tenant_id IS NULL;

CREATE INDEX platform_assignments_lookup
    ON platform_assignments (operator_id, tenant_id, revoked_at);

-- ---------------------------------------------------------------------------
-- The platform tenant is gone with the model that needed it.
-- ---------------------------------------------------------------------------
DROP INDEX IF EXISTS tenants_one_platform;
ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_platform_key;
ALTER TABLE tenants DROP COLUMN IF EXISTS is_platform;

COMMENT ON TABLE platform_users IS
    'Who operates this installation (ADR-0011). Deliberately unrelated to '
    'identities: a tenant user is not an operator and cannot become one.';
COMMENT ON TABLE platform_assignments IS
    'Which tenants a platform user may administer. Never a substitute for a '
    'grant, which is what a tenant''s own members hold.';
