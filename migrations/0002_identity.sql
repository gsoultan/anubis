-- ============================================================================
-- 0002_identity.sql — identities, credentials, sessions, tokens, signing keys
-- ============================================================================

CREATE TABLE identities (
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    disabled_at  timestamptz,
    last_login_at timestamptz,
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    -- Global kill switch. Bumping this invalidates every outstanding token
    -- for this identity without touching a single token row.
    token_epoch  int  NOT NULL DEFAULT 1,
    status       text NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active','disabled','locked','pending')),
    username     text NOT NULL,
    email        text,
    external_ref text,
    attributes   jsonb NOT NULL DEFAULT '{}',

    UNIQUE (id, tenant_id)   -- composite-FK target
) WITH (fillfactor = 85);    -- last_login_at/token_epoch update in place

-- citext avoided (extension); lower() expression index instead.
CREATE UNIQUE INDEX identities_username ON identities (tenant_id, lower(username));
CREATE UNIQUE INDEX identities_email    ON identities (tenant_id, lower(email))
    WHERE email IS NOT NULL;
CREATE UNIQUE INDEX identities_extref   ON identities (tenant_id, external_ref)
    WHERE external_ref IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Credentials — one row per authentication factor.
-- Password, device key (biometric), API key, TOTP all share this table.
-- Adding a factor type never changes the schema.
-- ---------------------------------------------------------------------------
CREATE TABLE credentials (
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    expires_at   timestamptz,
    revoked_at   timestamptz,
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    identity_id  uuid NOT NULL,
    tenant_id    uuid NOT NULL,
    -- WebAuthn/device-key replay defence: must increase monotonically
    sign_counter bigint NOT NULL DEFAULT 0,
    kind         text NOT NULL
                 CHECK (kind IN ('password','device_key','api_key','totp','recovery_code','oidc_link')),
    -- password: '$pbkdf2-sha256$i=600000$<salt>$<hash>'  (algo+params inline
    --           so the KDF can be upgraded by rehashing on next login)
    -- api_key / refresh-like: sha256 hex
    -- device_key: raw ed25519 public key, base64url
    secret       text,
    -- indexed lookup prefix for api keys: 'anb_live_<prefix>'
    lookup_key   text,
    label        text,
    params       jsonb NOT NULL DEFAULT '{}',

    FOREIGN KEY (identity_id, tenant_id) REFERENCES identities(id, tenant_id) ON DELETE CASCADE
) WITH (fillfactor = 85);

CREATE INDEX credentials_identity ON credentials (identity_id, kind)
    WHERE revoked_at IS NULL;
-- API-key auth is a single index probe, never a table scan
CREATE UNIQUE INDEX credentials_lookup ON credentials (lookup_key)
    WHERE lookup_key IS NOT NULL AND revoked_at IS NULL;
-- At most one active password per identity
CREATE UNIQUE INDEX credentials_one_password ON credentials (identity_id)
    WHERE kind = 'password' AND revoked_at IS NULL;

-- ---------------------------------------------------------------------------
-- Sessions — the anchor. Sign-out revokes here; everything else follows.
-- ---------------------------------------------------------------------------
CREATE TABLE sessions (
    created_at     timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now(),
    auth_time      timestamptz NOT NULL DEFAULT now(),  -- for step-up recency
    expires_at     timestamptz NOT NULL,
    revoked_at     timestamptz,
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    identity_id    uuid NOT NULL,
    tenant_id      uuid NOT NULL,
    application_id uuid,
    -- how they authenticated: {pwd}, {pwd,otp}, {device_key}
    amr            text[] NOT NULL DEFAULT '{}',
    revoke_reason  text,
    device_fp      text,
    ip             inet,
    user_agent     text,
    -- active scope context per axis: {"org":"<uuid>","product":"<uuid>"}
    active_scopes  jsonb NOT NULL DEFAULT '{}',

    FOREIGN KEY (identity_id, tenant_id) REFERENCES identities(id, tenant_id) ON DELETE CASCADE,
    FOREIGN KEY (application_id, tenant_id) REFERENCES applications(id, tenant_id) ON DELETE CASCADE,
    UNIQUE (id, tenant_id)
) WITH (fillfactor = 80);   -- last_seen_at updates on every refresh: HOT matters

CREATE INDEX sessions_identity ON sessions (identity_id, created_at DESC)
    WHERE revoked_at IS NULL;
-- reaper: only ever scans live rows
CREATE INDEX sessions_expiry ON sessions (expires_at) WHERE revoked_at IS NULL;

-- ---------------------------------------------------------------------------
-- Refresh tokens — PARTITIONED BY expires_at.
--
-- Retention becomes DROP PARTITION (instant, no bloat) instead of a bulk
-- DELETE that leaves dead tuples for autovacuum to grind through. This table
-- has the highest churn in the system; getting this wrong is how auth
-- databases end up 400GB of bloat.
--
-- Rotation model: each row is single-use. On refresh the row is marked
-- 'consumed' and a successor is issued. Presenting a consumed token means
-- THEFT -> revoke the whole family_id.
-- ---------------------------------------------------------------------------
CREATE TABLE refresh_tokens (
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    consumed_at  timestamptz,
    revoked_at   timestamptz,
    id           uuid NOT NULL DEFAULT uuidv7(),
    session_id   uuid NOT NULL,
    tenant_id    uuid NOT NULL,
    family_id    uuid NOT NULL,      -- all descendants of one login
    successor_id uuid,
    generation   int  NOT NULL DEFAULT 0,
    status       text NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active','consumed','revoked')),
    -- sha256 of the opaque token. Never store the token itself.
    token_hash   bytea NOT NULL,
    -- optional proof-of-possession public key (DPoP-style binding)
    bound_key    text,

    PRIMARY KEY (id, expires_at)
) PARTITION BY RANGE (expires_at);

-- Lookup is BY HASH, so this must be unique and fast across partitions.
CREATE UNIQUE INDEX refresh_tokens_hash ON refresh_tokens (token_hash, expires_at);
CREATE INDEX refresh_tokens_family ON refresh_tokens (family_id)
    WHERE status = 'active';
CREATE INDEX refresh_tokens_session ON refresh_tokens (session_id);

-- ---------------------------------------------------------------------------
-- Signing keys. Private key encrypted with a KMS-held master key.
-- kid is looked up ONLY from a preloaded in-memory map (never a DB query on
-- the verify path) — attacker-controlled input must not drive I/O.
-- ---------------------------------------------------------------------------
CREATE TABLE signing_keys (
    created_at    timestamptz NOT NULL DEFAULT now(),
    not_before    timestamptz NOT NULL,
    not_after     timestamptz NOT NULL,
    retired_at    timestamptz,
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    kid           text NOT NULL UNIQUE,
    alg           text NOT NULL DEFAULT 'Ed25519' CHECK (alg IN ('Ed25519','ES256')),
    status        text NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending','active','retiring','retired')),
    purpose       text NOT NULL DEFAULT 'access'
                  CHECK (purpose IN ('access','local')),
    public_key    bytea NOT NULL,
    private_key_enc bytea NOT NULL,
    kms_key_ref   text
);

-- Exactly one active signing key per purpose at any time.
CREATE UNIQUE INDEX signing_keys_one_active ON signing_keys (purpose)
    WHERE status = 'active';
