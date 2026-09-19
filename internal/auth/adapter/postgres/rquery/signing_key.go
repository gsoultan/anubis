package authrquery

import "github.com/gsoultan/storm"

// The rotation is three statements and an order: promote the pending key,
// demote the active one, retire the demoted one when the last token it signed
// has expired. Each is a guarded transition on a STATUS, which is why none of
// them is a builder — the guard names the state it is leaving.

// SetSigningKeyStatus moves one key by kid.
//
// retired_at is stamped only on the transition INTO retired, and left alone
// otherwise: a key moved from retiring back to active must not keep a
// retirement date, and re-retiring one must not move the date it actually
// stopped signing.
var SetSigningKeyStatus = storm.SQLExec(`
UPDATE signing_keys
SET status = $2,
    retired_at = CASE WHEN $2 = 'retired' THEN now() ELSE retired_at END
WHERE kid = $1`)

// PromotePendingKey makes the published key the signing one.
//
// By PURPOSE and status rather than by kid: the caller is rotating "the access
// key", and naming the kid would mean reading it first — a window in which a
// concurrent rotation could promote a different one.
var PromotePendingKey = storm.SQLExec(`
UPDATE signing_keys SET status = 'active'
WHERE purpose = $1 AND status = 'pending'`)

// DemoteActiveKey moves the signing key to retiring, where it still VERIFIES
// until the last token signed with it expires.
var DemoteActiveKey = storm.SQLExec(`
UPDATE signing_keys SET status = 'retiring'
WHERE purpose = $1 AND status = 'active'`)
