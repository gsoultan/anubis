-- ============================================================================
-- 0030 — API keys belong to the tenant, not to a person.
--
-- An API key is machine access: a gateway asking authorize(), an integration
-- reading the decision API. Storing it as a person's credential tied its
-- lifetime to that person — disable the person who happened to create it and
-- the tenant's integration dies with them — and it misfiled what the key IS:
-- the caller is the tenant's system, not anybody's identity.
--
-- So keys move to their own tenant-scoped table, created by platform users
-- (the only population that administers anything, 0029). A person can no
-- longer hold one at all: 'api_key' leaves the credentials CHECK, making the
-- old shape unstorable rather than merely deprecated.
-- ============================================================================

CREATE TABLE api_keys (
    created_at   timestamptz NOT NULL DEFAULT now(),
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    label        text NOT NULL DEFAULT '',
    -- Indexed probe: 'anb_live_<prefix>_<secret>' splits, the lookup finds the
    -- row, the secret is compared against the hash. Same scheme as before.
    lookup       text NOT NULL,
    -- sha256 hex of the secret half. Never the secret.
    secret_hash  text NOT NULL,
    -- A platform user, never an identity: operators create machine access.
    created_by   uuid REFERENCES platform_users(id) ON DELETE SET NULL,
    last_used_at timestamptz,
    expires_at   timestamptz,
    revoked_at   timestamptz
);

-- Auth is a single index probe, never a scan.
CREATE UNIQUE INDEX api_keys_lookup ON api_keys (lookup)
    WHERE revoked_at IS NULL;
CREATE INDEX api_keys_tenant ON api_keys (tenant_id, revoked_at);

-- Carry existing keys across so none stop working mid-flight. Whoever's
-- identity they hung off is not recorded — that linkage is the mistake being
-- removed.
INSERT INTO api_keys (created_at, id, tenant_id, label, lookup, secret_hash,
                      last_used_at, expires_at, revoked_at)
SELECT c.created_at, c.id, c.tenant_id, COALESCE(c.label, ''), c.lookup_key,
       c.secret, c.last_used_at, c.expires_at, c.revoked_at
  FROM credentials c
 WHERE c.kind = 'api_key' AND c.lookup_key IS NOT NULL AND c.secret IS NOT NULL;

DELETE FROM credentials WHERE kind = 'api_key';

-- A person's credentials are how a PERSON proves themself. Machine access is
-- not among them anymore, by constraint rather than by convention.
ALTER TABLE credentials DROP CONSTRAINT IF EXISTS credentials_kind_check;
ALTER TABLE credentials ADD CONSTRAINT credentials_kind_check
    CHECK (kind IN ('password','device_key','totp','recovery_code','oidc_link'));

COMMENT ON TABLE api_keys IS
    'The tenant''s machine credentials (ADR-0011). Created by platform users; '
    'authenticate as the tenant''s system, never as any person.';
