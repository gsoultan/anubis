-- ============================================================================
-- 0027 — A second factor for platform users (ADR-0011).
--
-- These are the accounts that run the installation: they create tenants,
-- appoint operators, and reach into any tenant they are assigned to. Until
-- now they authenticated with a password alone, while a tenant's own realms
-- could demand TOTP of ordinary users — the wrong way round.
--
-- The rule matches the one already in force for identities: an ENROLLED
-- factor is always demanded. There is deliberately no "required but not
-- enrolled" policy flip, because turning that on would lock out every
-- operator who had not got round to enrolling, including the only owner.
-- ============================================================================

ALTER TABLE platform_users
    -- Sealed with the master key, bound to this row's id as additional data,
    -- exactly as signing keys are sealed. A dump of this table without the
    -- master key yields nothing that can generate a code.
    ADD COLUMN totp_secret_enc  bytea,
    -- NULL until a code has been verified. Holding a secret is not the same
    -- as having enrolled: a half-finished enrolment must never start
    -- demanding a factor the operator cannot produce.
    ADD COLUMN totp_enrolled_at timestamptz,
    -- The last TOTP step accepted for this account. A step may be accepted
    -- once and only once, so a code observed in flight cannot be replayed
    -- inside its own validity window.
    ADD COLUMN totp_last_step   bigint NOT NULL DEFAULT 0;

-- An enrolled account must have something to verify against. The pair can
-- only ever move together, so a row can never be in the state that demands a
-- factor it cannot check.
ALTER TABLE platform_users
    ADD CONSTRAINT platform_users_totp_complete
    CHECK (totp_enrolled_at IS NULL OR totp_secret_enc IS NOT NULL);

COMMENT ON COLUMN platform_users.totp_secret_enc IS
    'AES-256-GCM under the master key, AAD = platform_users.id.';
COMMENT ON COLUMN platform_users.totp_last_step IS
    'Monotonic single-use guard: a TOTP step is accepted at most once.';
