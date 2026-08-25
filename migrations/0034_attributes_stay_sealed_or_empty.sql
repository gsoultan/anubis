-- ============================================================================
-- 0034 — identities.attributes may not hold plaintext.
--
-- ADR-0013 decided that `attributes` is the ONE column worth encrypting: it
-- is the only one whose contents are arbitrary (a home address, a date of
-- birth, a case note) and the only one no query depends on. The sealing
-- machinery exists (0022, internal/identity/domain/pii) and the shredding
-- path is wired to retention.
--
-- What does not exist is the WRITE path. No endpoint populates this column,
-- so today it is empty on every row and the encryption guards nothing. That
-- is a benign state and a fragile one: the moment somebody adds a write —
-- an importer field, a custom attribute in the console — it lands in
-- plaintext, in the one column this project promised to encrypt, and nothing
-- would notice.
--
-- So make the promise structural rather than aspirational. Until sealing
-- lands, an attributes value that is not empty is UNSTORABLE. Whoever builds
-- the write path has to drop this constraint, and in doing so has to read
-- ADR-0013 and decide deliberately — which is the entire point.
-- ============================================================================

-- Every row is already '{}' (verified: 0 of 57,012 non-empty on the
-- development database), so this validates immediately.
ALTER TABLE identities
    ADD CONSTRAINT identities_attributes_not_plaintext
    CHECK (attributes = '{}'::jsonb);

COMMENT ON CONSTRAINT identities_attributes_not_plaintext ON identities IS
    'ADR-0013: attributes is the column marked for per-field sealing. Until '
    'the sealing write path exists, a non-empty value here would be plaintext '
    'PII. Drop this constraint as part of shipping that path, not before.';
