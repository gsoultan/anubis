package authrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// APIKeyAuthRow is a presented key with the tenant facts every request
// re-checks — a key belonging to a suspended tenant must stop working at the
// same moment the tenant does, which is why the status is read here and not
// trusted from when the key was minted.
type APIKeyAuthRow struct {
	ID           string
	TenantID     string
	Label        string
	SecretHash   string
	ExpiresAt    runtime.Null[time.Time]
	RevokedAt    runtime.Null[time.Time]
	TenantSlug   string
	TenantStatus string
}

var GetAPIKeyByLookup = storm.SQL[APIKeyAuthRow](`
SELECT k.id::text AS id, k.tenant_id::text AS tenant_id, k.label, k.secret_hash,
       k.expires_at, k.revoked_at, t.slug AS tenant_slug, t.status AS tenant_status
  FROM api_keys k
  JOIN tenants t ON t.id = k.tenant_id
 WHERE k.lookup = $1 AND k.revoked_at IS NULL`)

// APIKeyListRow is one tenant API key, with its creator resolved by name.
type APIKeyListRow struct {
	ID         string
	Label      string
	Lookup     string
	CreatedAt  time.Time
	LastUsedAt runtime.Null[time.Time]
	ExpiresAt  runtime.Null[time.Time]
	RevokedAt  runtime.Null[time.Time]
	CreatedBy  string
}

// ListAPIKeys shows who created each key by NAME: an audit question.
//
// The creator is a PLATFORM user, so the join goes there and nowhere near
// identities — a tenant's own people cannot mint these. LEFT, because a key
// made by the bootstrap has no creator.
var ListAPIKeys = storm.SQL[APIKeyListRow](`
SELECT k.id::text AS id, k.label, k.lookup, k.created_at, k.last_used_at,
       k.expires_at, k.revoked_at, COALESCE(u.username, '') AS created_by
  FROM api_keys k
  LEFT JOIN platform_users u ON u.id = k.created_by
 WHERE k.tenant_id = $1
 ORDER BY k.created_at DESC`)

// RevokeAPIKey revokes a live key; the guard makes the row count meaningful.
var RevokeAPIKey = storm.SQLExec(`
UPDATE api_keys SET revoked_at = now()
 WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL`)
