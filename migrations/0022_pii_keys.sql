-- ============================================================================
-- 0022_pii_keys.sql — crypto-shredding key management
--
-- Applicant and customer data cannot be kept indefinitely (UU PDP 27/2022 and
-- equivalents), which conflicts with "never delete, for audit". The schema has
-- carried identities.pii_key_id since 0008 as the resolution — PII stored under
-- a per-identity key — but nothing minted or destroyed those keys.
--
-- This is that missing half. Each identity in a pii_encryption realm gets a row
-- here; the key material is sealed with the KMS-held master key exactly like
-- signing keys (never plaintext at rest, never in a backup). ERASURE DELETES
-- THIS ROW. The ciphertext elsewhere becomes permanently unreadable while the
-- identity row, its grants and the audit chain survive intact — referential
-- integrity preserved, personal data gone.
--
-- ON DELETE SET NULL, deliberately: shredding the key must never cascade into
-- deleting the identity. Losing the audit trail is the outcome this whole
-- design exists to avoid.
-- ============================================================================

CREATE TABLE pii_keys (
    created_at  timestamptz NOT NULL DEFAULT now(),
    shredded_at timestamptz,               -- set on the tombstone, see below
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    -- nonce(12) || AES-256-GCM(HKDF(master,"anubis/pii/v1"), key), AAD = id
    key_enc     bytea NOT NULL,
    kms_key_ref text
);

CREATE INDEX pii_keys_tenant ON pii_keys (tenant_id);

ALTER TABLE identities
    ADD CONSTRAINT identities_pii_key_fk
    FOREIGN KEY (pii_key_id) REFERENCES pii_keys(id) ON DELETE SET NULL;

-- ---------------------------------------------------------------------------
-- Shredding is a DELETE, and a delete leaves no evidence that it happened.
-- The tombstone records that a key existed and was destroyed — the fact of
-- erasure is itself auditable, without retaining anything erased.
-- ---------------------------------------------------------------------------
CREATE TABLE pii_key_tombstones (
    shredded_at timestamptz NOT NULL DEFAULT now(),
    key_id      uuid PRIMARY KEY,
    tenant_id   uuid NOT NULL,
    reason      text NOT NULL CHECK (reason IN ('erasure_request','retention','admin'))
);

CREATE OR REPLACE FUNCTION pii_shred(p_key uuid, p_reason text)
RETURNS boolean LANGUAGE plpgsql AS $fn$
DECLARE v_tenant uuid;
BEGIN
    SELECT tenant_id INTO v_tenant FROM pii_keys WHERE id = p_key;
    IF v_tenant IS NULL THEN
        RETURN false;   -- already shredded; erasure is idempotent by design
    END IF;
    INSERT INTO pii_key_tombstones (key_id, tenant_id, reason)
         VALUES (p_key, v_tenant, p_reason)
    ON CONFLICT (key_id) DO NOTHING;
    DELETE FROM pii_keys WHERE id = p_key;
    RETURN true;
END;
$fn$;
