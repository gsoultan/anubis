package gaterquery

import (
	"time"

	"github.com/gsoultan/storm"
)

// ShareRow is one live record share, as the snapshot carries it: who may do
// what on which record, until when. The sharer is not needed to decide.
type ShareRow struct {
	ToIdentityID   string
	Record         string
	PermissionKeys []string
	ValidUntil     time.Time
}

// Live only, like SnapshotGrants: a revoked or lapsed share is not a share.
// valid_until is still carried so the evaluator can age it between rebuilds.
var SnapshotShares = storm.SQL[ShareRow](`
SELECT s.to_identity_id::text AS to_identity_id, s.record, s.permission_keys, s.valid_until
FROM record_shares s
WHERE s.tenant_id = $1 AND s.revoked_at IS NULL AND s.valid_until > now()`)
