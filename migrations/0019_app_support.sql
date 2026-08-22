-- ============================================================================
-- 0019_app_support.sql — server-side single-use state for the application layer
--
-- Three application flows need state that must be consumed EXACTLY ONCE:
--   * MFA challenge tokens        (60s, one verify attempt consumes)
--   * OIDC authorization codes    (PKCE; a code exchanged twice is an attack)
--   * device-key login nonces     (a nonce verified twice is a replay)
-- plus email verification and password reset later — same shape.
--
-- One table covers all of them. Atomic consumption is
--     DELETE ... WHERE token_hash = $1 AND expires_at > now() RETURNING ...
-- which has GETDEL semantics without adding Redis (ADR-0002 posture: no new
-- infrastructure until multi-instance deployment demands shared state).
-- Only the sha256 of the presented secret is stored, same rule as
-- refresh_tokens: a database read never yields a usable credential.
-- ============================================================================

CREATE TABLE one_time_tokens (
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    id         uuid NOT NULL DEFAULT uuidv7(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kind       text NOT NULL CHECK (kind IN
               ('mfa','auth_code','device_challenge','email_verify','password_reset')),
    token_hash bytea NOT NULL,
    -- flow state, snapshotted at issue time. For auth_code:
    --   {identity_id, session_id, client_id, redirect_uri, code_challenge,
    --    code_challenge_method, scope, nonce}
    payload    jsonb NOT NULL DEFAULT '{}',

    PRIMARY KEY (id),
    UNIQUE (token_hash)
);

-- reaper: DELETE WHERE expires_at < now() on a schedule; low volume, no
-- partitioning needed (unlike refresh_tokens these live seconds to minutes)
CREATE INDEX one_time_tokens_expiry ON one_time_tokens (expires_at);

-- ---------------------------------------------------------------------------
-- Browser SSO cookie. The __Host-anubis_sso cookie value is an opaque 256-bit
-- secret; only its sha256 lands here. A cookie can only ever reference a live
-- session — revoking the session kills the cookie with it, which is exactly
-- the single-logout behaviour the browser flow requires.
-- ---------------------------------------------------------------------------
ALTER TABLE sessions ADD COLUMN cookie_hash bytea;

CREATE UNIQUE INDEX sessions_cookie ON sessions (cookie_hash)
    WHERE cookie_hash IS NOT NULL AND revoked_at IS NULL;

-- ---------------------------------------------------------------------------
-- Audit hash chain: the writer needs "last (seq, entry_hash) for this tenant"
-- as one index probe. The existing indexes cover (tenant, action, time) but
-- not (tenant, seq).
-- ---------------------------------------------------------------------------
CREATE INDEX audit_log_tenant_seq ON audit_log (tenant_id, seq DESC);
