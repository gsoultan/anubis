-- ============================================================================
-- 0031 — Refresh for platform sessions (rotation + theft detection).
--
-- Operators had 1-hour tokens and nothing else: the console re-asked for a
-- password every hour, which trains people to type it reflexively — the
-- opposite of what MFA-carrying accounts deserve. The mechanism mirrors the
-- identity side's hard-won semantics at operator scale:
--
--   * a refresh token is SINGLE-USE; using it mints a successor in the same
--     family and marks the old one consumed, atomically;
--   * presenting a consumed (or revoked) token is theft, not error: the
--     WHOLE family dies at once and the event is audited as
--     token.reuse_detected — the pager action;
--   * a family has an ABSOLUTE lifetime from first sign-in. Rotation never
--     extends it: after that, the password (and factor) again.
--
-- Storage carries only sha256 of the secret — a dump of this table cannot
-- be replayed.
-- ============================================================================

CREATE TABLE platform_refresh_tokens (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    platform_user_id uuid        NOT NULL REFERENCES platform_users(id) ON DELETE CASCADE,
    -- The family is the sign-in: the first token's own id, inherited by
    -- every successor. Revocation is by family, never by single row.
    family_id        uuid        NOT NULL,
    token_hash       bytea       NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    -- Absolute: successors inherit the family's expiry unchanged.
    expires_at       timestamptz NOT NULL,
    used_at          timestamptz,
    revoked_at       timestamptz,

    CONSTRAINT platform_refresh_hash_len CHECK (octet_length(token_hash) = 32)
);

CREATE UNIQUE INDEX platform_refresh_by_hash
    ON platform_refresh_tokens (token_hash);
CREATE INDEX platform_refresh_by_family
    ON platform_refresh_tokens (family_id);

-- Least privilege (0023): the runtime reads and writes these rows; nobody
-- else needs them, and readonly must not see even hashes.
GRANT SELECT, INSERT, UPDATE, DELETE ON platform_refresh_tokens TO anubis_app;
