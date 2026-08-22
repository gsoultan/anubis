-- ============================================================================
-- 0008_realms.sql — EXTERNAL IDENTITY POPULATIONS
--
-- Anubis serves employees, business partners (suppliers, vendors, contractors)
-- and public users (job applicants, customers). These differ in ways that are
-- NOT expressible as scope axes:
--
--   * how an account comes into existence  (HR sync / contract / self-service)
--   * which authentication factors apply   (SSO+TOTP / TOTP / email OTP)
--   * how strongly identity is verified    (government ID / contract / email)
--   * who administers the account          (IT / the partner itself / the user)
--   * how long data may be retained        (employment records / contract term /
--                                           LEGALLY BOUNDED, must be deleted)
--
-- REJECTED: giving each partner its own tenant.
--   A supplier user needs access to OUR purchase-order data, which would require
--   cross-tenant grants. Every grant/scope FK in this schema is deliberately
--   composite on tenant_id precisely to make cross-tenant references impossible
--   to insert -- the single strongest control in the design. Relaxing it to
--   support suppliers would trade a real security boundary for a modelling
--   convenience.
--
-- CHOSEN: tenant stays the isolation boundary; REALM partitions the population
--   inside it. Partner organisations become scope nodes on a 'partner' axis, so
--   authorization keeps working exactly as designed with no new mechanism.
-- ============================================================================

CREATE TABLE realms (
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    session_ttl       interval NOT NULL DEFAULT '12 hours',
    access_token_ttl  interval NOT NULL DEFAULT '10 minutes',
    refresh_token_ttl interval NOT NULL DEFAULT '30 days',
    -- Applicant data cannot be kept indefinitely (UU PDP 27/2022 and
    -- equivalents). NULL means no statutory limit.
    default_retention interval,
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    -- NIST 800-63 IAL, as an ordered smallint so it compares cheaply:
    --   1 = self-asserted   2 = remotely verified   3 = in-person verified
    min_assurance     smallint NOT NULL DEFAULT 1 CHECK (min_assurance BETWEEN 1 AND 3),
    self_registration boolean NOT NULL DEFAULT false,
    email_verification_required boolean NOT NULL DEFAULT true,
    -- PII stored under a per-identity key so deletion can be crypto-shredding:
    -- drop the key and the data is unrecoverable while rows and referential
    -- integrity survive for audit.
    pii_encryption    boolean NOT NULL DEFAULT false,
    kind              text NOT NULL
                      CHECK (kind IN ('employee','partner','public','service')),
    code              text NOT NULL CHECK (code ~ '^[a-z][a-z0-9_]{1,30}$'),
    display_name      text NOT NULL,
    allowed_factors   text[] NOT NULL DEFAULT '{password}',
    required_factors  text[] NOT NULL DEFAULT '{password}',
    password_policy   jsonb NOT NULL DEFAULT '{}',

    UNIQUE (tenant_id, code),
    UNIQUE (id, tenant_id)          -- composite-FK target
);

-- ---------------------------------------------------------------------------
-- identities gains realm membership, assurance, and a privacy lifecycle
-- ---------------------------------------------------------------------------
ALTER TABLE identities
    ADD COLUMN realm_id           uuid,
    ADD COLUMN assurance_level    smallint NOT NULL DEFAULT 1
               CHECK (assurance_level BETWEEN 1 AND 3),
    ADD COLUMN retention_until    timestamptz,
    ADD COLUMN deletion_requested_at timestamptz,
    ADD COLUMN anonymized_at      timestamptz,
    ADD COLUMN pii_key_id         uuid;

ALTER TABLE identities
    ADD CONSTRAINT identities_realm_fk
    FOREIGN KEY (realm_id, tenant_id) REFERENCES realms(id, tenant_id) ON DELETE RESTRICT;

-- Username uniqueness moves INSIDE the realm. 'alice' the employee and
-- 'alice' the job applicant are different people and must not collide.
DROP INDEX identities_username;
DROP INDEX identities_email;
CREATE UNIQUE INDEX identities_username ON identities (tenant_id, realm_id, lower(username));
CREATE UNIQUE INDEX identities_email    ON identities (tenant_id, realm_id, lower(email))
    WHERE email IS NOT NULL;

-- Retention sweeper: only ever scans rows with a deadline.
CREATE INDEX identities_retention ON identities (retention_until)
    WHERE retention_until IS NOT NULL AND anonymized_at IS NULL;

-- ---------------------------------------------------------------------------
-- identity_links — the same human across realms.
-- A supplier contact who is later hired, or an applicant who becomes an
-- employee, must not silently inherit the old account's access. Linking is
-- explicit, evidenced, and never merges grants.
-- ---------------------------------------------------------------------------
CREATE TABLE identity_links (
    linked_at    timestamptz NOT NULL DEFAULT now(),
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id    uuid NOT NULL,
    primary_id   uuid NOT NULL,
    secondary_id uuid NOT NULL,
    linked_by    uuid NOT NULL,
    method       text NOT NULL CHECK (method IN ('manual','email_proof','document','hr_sync')),
    evidence     jsonb NOT NULL DEFAULT '{}',

    FOREIGN KEY (primary_id, tenant_id)   REFERENCES identities(id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (secondary_id, tenant_id) REFERENCES identities(id, tenant_id) ON DELETE CASCADE,
    CHECK (primary_id <> secondary_id),
    UNIQUE (primary_id, secondary_id)
);

-- ---------------------------------------------------------------------------
-- consents — lawful basis for processing external users' personal data.
-- Append-only: a withdrawal is a new row, never an update, so the record of
-- what was consented to and when survives.
-- ---------------------------------------------------------------------------
CREATE TABLE consents (
    granted_at   timestamptz NOT NULL DEFAULT now(),
    withdrawn_at timestamptz,
    expires_at   timestamptz,
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id    uuid NOT NULL,
    identity_id  uuid NOT NULL,
    purpose      text NOT NULL,     -- 'recruitment','marketing','contract_performance'
    policy_version text NOT NULL,
    evidence     jsonb NOT NULL DEFAULT '{}',   -- ip, user agent, timestamp shown

    FOREIGN KEY (identity_id, tenant_id) REFERENCES identities(id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX consents_identity ON consents (identity_id, purpose) WHERE withdrawn_at IS NULL;

-- ---------------------------------------------------------------------------
-- Self-scoped grants: "you may read applications, but only your own."
--
-- This is the dominant external-user access shape and it is NOT a scope
-- question -- no tree node describes "the row you own". Rather than open the
-- door to general ABAC (deferred, see ADR-0004), one narrow well-defined
-- concept covers the majority of real cases.
--
-- The caller passes the record's owner as the reserved target key '_owner'.
-- Reserved keys start with '_', which scope_axes.code CHECK forbids, so an
-- axis can never collide with one.
-- ---------------------------------------------------------------------------
ALTER TABLE grants ADD COLUMN self_scoped boolean NOT NULL DEFAULT false;

-- ---------------------------------------------------------------------------
-- Permissions may demand a minimum identity assurance.
-- A self-registered applicant must not approve a purchase order even if a
-- misconfigured grant says otherwise. Defence in depth against grant errors.
-- ---------------------------------------------------------------------------
ALTER TABLE permissions
    ADD COLUMN min_assurance smallint NOT NULL DEFAULT 1
    CHECK (min_assurance BETWEEN 1 AND 3);

-- ---------------------------------------------------------------------------
-- role_grantable — delegated administration WITHOUT privilege escalation.
--
-- Partner admins manage their own users. Without a restriction, a supplier
-- admin could grant themselves finance:payment:approve. A role may only confer
-- roles explicitly listed here.
-- ---------------------------------------------------------------------------
CREATE TABLE role_grantable (
    role_id      uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    grantable_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, grantable_id),
    CHECK (role_id <> grantable_id)
);

-- Restrict which realms a role may be granted in: an 'employee.finance' role
-- must never be attachable to a public self-registered account.
ALTER TABLE roles ADD COLUMN allowed_realm_kinds text[] NOT NULL DEFAULT '{employee}';

CREATE TRIGGER realms_bump_ins AFTER INSERT ON realms
    REFERENCING NEW TABLE AS newtab
    FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt();
CREATE TRIGGER realms_bump_upd AFTER UPDATE ON realms
    REFERENCING NEW TABLE AS newtab
    FOR EACH STATEMENT EXECUTE FUNCTION trg_bump_catalog_stmt();
