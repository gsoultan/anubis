package identityrquery

import "github.com/gsoultan/storm"

// WithdrawConsent stamps the row it withdraws. Guarded on withdrawn_at, so the
// row count distinguishes "I withdrew it" from "it already was".
var WithdrawConsent = storm.SQLExec(`
UPDATE consents SET withdrawn_at = now()
WHERE id = $1 AND tenant_id = $2 AND withdrawn_at IS NULL`)
