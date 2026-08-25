-- ============================================================================
-- 0033 — Machine credentials for platform users.
--
-- Migration 0029 made administration operator-only, which killed the one
-- automated path that mattered: applying an application's manifest from CI.
-- API keys had belonged to tenant identities, and tenant identities can no
-- longer administer anything, so the capability went with them.
--
-- This restores it honestly. A platform API key acts AS the operator who
-- owns it, carrying exactly their assignments — checked on every call, so
-- revoking somebody's access revokes their pipeline's too, at the same
-- moment. No key can do more than the person who created it.
-- ============================================================================

CREATE TABLE platform_api_keys (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    -- The owner. Deleting an operator takes their machine credentials with
    -- them: a key that outlived its owner answers to nobody.
    platform_user_id uuid NOT NULL REFERENCES platform_users(id) ON DELETE CASCADE,
    label            text NOT NULL DEFAULT '',
    -- Same wire format as tenant keys: 'anb_live_<lookup>_<secret>'. The
    -- lookup finds the row; the secret is compared against the hash.
    lookup           text NOT NULL,
    -- sha256 hex of the secret half. Never the secret itself.
    secret_hash      text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid REFERENCES platform_users(id) ON DELETE SET NULL,
    last_used_at     timestamptz,
    -- An expiry is REQUIRED. A tenant's key may reasonably live until it is
    -- revoked; a credential that administers the whole installation should
    -- not outlive the reason it was made, and an unbounded one always does.
    expires_at       timestamptz NOT NULL,
    revoked_at       timestamptz
);

-- Auth is a single index probe, never a scan. Revoked keys leave the index
-- so revocation takes effect on the next request.
CREATE UNIQUE INDEX platform_api_keys_lookup ON platform_api_keys (lookup)
    WHERE revoked_at IS NULL;
CREATE INDEX platform_api_keys_owner ON platform_api_keys (platform_user_id, revoked_at);

GRANT SELECT, INSERT, UPDATE ON platform_api_keys TO anubis_app;
-- readonly must not see even a hash of a credential.
