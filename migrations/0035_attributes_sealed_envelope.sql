-- ============================================================================
-- 0035 — the sealing write path exists, so attributes may hold ciphertext.
--
-- 0034 made a non-empty `identities.attributes` unstorable, on purpose: the
-- column was marked for encryption by ADR-0013, the sealing machinery was
-- built, and nothing connected the two. A constraint was the honest way to
-- say "this promise is not kept yet" — and to force whoever kept it to read
-- the ADR first.
--
-- This is that commit. `SetIdentityAttributes` seals the whole attribute map
-- under the identity's own key (minted on first write, shredded by retention)
-- and stores the envelope below. So the tripwire is replaced rather than
-- simply dropped: plaintext is still unstorable, but ciphertext now fits.
--
--     {"v": 1, "sealed": "<base64url of nonce||ciphertext>"}
--
-- The check is structural, not cryptographic — it cannot tell a real
-- ciphertext from a base64-looking string, and it is not trying to. What it
-- catches is the realistic accident: a future writer that forgets to seal and
-- puts {"employee_id": "…"} straight into the column. That shape has no `v`
-- and no `sealed`, so the database refuses it.
-- ============================================================================

ALTER TABLE identities
    DROP CONSTRAINT identities_attributes_not_plaintext;

ALTER TABLE identities
    ADD CONSTRAINT identities_attributes_sealed
    CHECK (
        attributes = '{}'::jsonb
        OR (
            attributes ? 'v'
            AND attributes ? 'sealed'
            AND jsonb_typeof(attributes -> 'sealed') = 'string'
            AND length(attributes ->> 'sealed') > 0
        )
    );

COMMENT ON CONSTRAINT identities_attributes_sealed ON identities IS
    'ADR-0013: attributes holds either {} or a sealed envelope {"v","sealed"}. '
    'Plaintext PII in this column is a bug, and this refuses to store it.';
