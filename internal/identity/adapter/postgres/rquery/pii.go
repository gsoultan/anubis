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
