package authrquery

import (
	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// ConsumedTokenRow is the payload a one-time token carried.
type ConsumedTokenRow struct {
	ID       string
	TenantID string
	Payload  runtime.JSON
}

// ConsumeOneTimeToken is atomic single use: DELETE ... RETURNING has GETDEL
// semantics, so a second presentation finds no row.
//
// GET-then-DEL would leave a replay window exactly as wide as the round trip
// between them — which is the window an attacker who has the token is racing
// for.
var ConsumeOneTimeToken = storm.SQL[ConsumedTokenRow](`
DELETE FROM one_time_tokens
WHERE token_hash = $1 AND kind = $2 AND expires_at > now()
RETURNING id::text AS id, tenant_id::text AS tenant_id, payload`)

// SweepOneTimeTokens clears the expired ones.
//
// An hour late, not immediately: a just-expired row still answers "was this
// presented too late?" for as long as somebody might present it.
var SweepOneTimeTokens = storm.SQLExec(`
DELETE FROM one_time_tokens WHERE expires_at < now() - interval '1 hour'`)
