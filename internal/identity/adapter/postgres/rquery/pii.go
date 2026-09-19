package identityrquery

import (
	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// ShreddedRow answers whether the key was still recoverable when asked.
type ShreddedRow struct {
	Shredded bool
}

// ShredPIIKey destroys the key an identity's attributes are sealed with.
//
// Idempotent: a second erasure of the same identity returns false rather than
// failing, because "already unrecoverable" is the requested outcome.
var ShredPIIKey = storm.SQL[ShreddedRow](`
SELECT pii_shred($1, $2) AS shredded`)

var SetIdentityPIIKey = storm.SQLExec(`
UPDATE identities SET pii_key_id = $3, updated_at = now()
WHERE id = $1 AND tenant_id = $2`)

// AttributesRow is the sealed envelope and the key that opens it.
type AttributesRow struct {
	Attributes runtime.JSON
	PiiKeyID   runtime.Null[string]
	KeyEnc     []byte
}

// GetIdentityAttributes reads the envelope and its key TOGETHER.
//
// Fetching them in two statements leaves a window in which retention shreds
// the key in between, and the caller then reports "corrupt" for what is
// actually a completed erasure.
var GetIdentityAttributes = storm.SQL[AttributesRow](`
SELECT i.attributes, i.pii_key_id::text AS pii_key_id, k.key_enc
FROM identities i
LEFT JOIN pii_keys k ON k.id = i.pii_key_id
WHERE i.id = $1 AND i.tenant_id = $2`)

var SetIdentityAttributes = storm.SQLExec(`
UPDATE identities SET attributes = $3, updated_at = now()
WHERE id = $1 AND tenant_id = $2`)

// ResealableRow is one PII key together with the identity whose id is its
// additional authenticated data.
type ResealableRow struct {
	KeyID      string
	IdentityID string
	KeyEnc     []byte
}

// ListResealablePIIKeys is the master-key rotation sweep.
//
// The join is not incidental: a PII key's ciphertext is bound to the identity
// that owns it ("pii:" || identity_id as AAD), and pii_keys does not carry
// that id. Resealing without it produces a blob that decrypts to nothing.
//
// Shredded keys are excluded. There is no material left to reseal, and the
// absence IS the erasure — re-sealing a NULL would be inventing a key for
// data somebody asked to have destroyed.
var ListResealablePIIKeys = storm.SQL[ResealableRow](`
SELECT k.id::text AS key_id, i.id::text AS identity_id, k.key_enc
FROM pii_keys k
JOIN identities i ON i.pii_key_id = k.id
WHERE k.key_enc IS NOT NULL AND k.shredded_at IS NULL
ORDER BY k.id`)

// ResealPIIKey replaces the sealed key material and records which master did
// it, so a re-run after a partial failure can tell the two apart.
var ResealPIIKey = storm.SQLExec(`
UPDATE pii_keys SET key_enc = $2, kms_key_ref = $3 WHERE id = $1`)
