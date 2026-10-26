package authzrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// ShareRow is one record share, as the service lists and checks it.
type ShareRow struct {
	ID             string
	TenantID       string
	FromIdentityID string
	ToIdentityID   string
	Record         string
	RecordScopes   runtime.JSON
	PermissionKeys []string
	Note           string
	CreatedAt      time.Time
	ValidUntil     time.Time
	RevokedAt      runtime.Null[time.Time]
	RevokeReason   runtime.Null[string]
}

const shareColumns = `
SELECT s.id::text AS id, s.tenant_id::text AS tenant_id,
       s.from_identity_id::text AS from_identity_id, s.to_identity_id::text AS to_identity_id,
       s.record, s.record_scopes, s.permission_keys, s.note,
       s.created_at, s.valid_until, s.revoked_at, s.revoke_reason
FROM record_shares s`

var CreateShare = storm.SQL[ShareRow](`
INSERT INTO record_shares (tenant_id, from_identity_id, to_identity_id, record,
                           record_scopes, permission_keys, note, valid_until)
VALUES ($1, $2, $3, $4, $5::jsonb, $6::text[], $7, $8)
RETURNING id::text AS id, tenant_id::text AS tenant_id,
          from_identity_id::text AS from_identity_id, to_identity_id::text AS to_identity_id,
          record, record_scopes, permission_keys, note,
          created_at, valid_until, revoked_at, revoke_reason`)

var GetShare = storm.SQL[ShareRow](shareColumns + `
WHERE s.id = $1 AND s.tenant_id = $2`)

// Live shares on one record, newest first. The caller decides who may see
// the list; this returns every share so that decision has what it needs.
var ListSharesOnRecord = storm.SQL[ShareRow](shareColumns + `
WHERE s.tenant_id = $1 AND s.record = $2 AND s.revoked_at IS NULL AND s.valid_until > now()
ORDER BY s.created_at DESC`)

var ListSharesForRecipient = storm.SQL[ShareRow](shareColumns + `
WHERE s.tenant_id = $1 AND s.to_identity_id = $2 AND s.revoked_at IS NULL AND s.valid_until > now()
ORDER BY s.created_at DESC`)

// RevokeShare is idempotent on an already-revoked row: the second revoke
// changes nothing and reports zero rows, which the caller reads as "done".
var RevokeShare = storm.SQLExec(`
UPDATE record_shares
   SET revoked_at = now(), revoke_reason = nullif($3, '')
 WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL`)
