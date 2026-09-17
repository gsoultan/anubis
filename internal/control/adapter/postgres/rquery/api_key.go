package controlrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// APIKeyLookupRow is a key and the owner facts every request re-checks.
type APIKeyLookupRow struct {
	ID             string
	PlatformUserID string
	SecretHash     string
	ExpiresAt      time.Time
	RevokedAt      runtime.Null[time.Time]
	Username       string
	OwnerStatus    string
	TokenEpoch     int32
}

// PlatformAPIKeyByLookup joins the owner so a disabled operator's key stops
// working at the same moment they do — the status is read on every request,
// not trusted from when the key was minted.
var PlatformAPIKeyByLookup = storm.SQL[APIKeyLookupRow](`
SELECT k.id::text AS id, k.platform_user_id::text AS platform_user_id,
       k.secret_hash, k.expires_at, k.revoked_at,
       u.username, u.status AS owner_status, u.token_epoch
FROM platform_api_keys k
JOIN platform_users u ON u.id = k.platform_user_id
WHERE k.lookup = $1 AND k.revoked_at IS NULL`)

// APIKeyListRow is one key as the console lists it. The secret hash is
// deliberately absent: nothing on a listing screen needs it.
type APIKeyListRow struct {
	ID             string
	PlatformUserID string
	Username       string
	Label          string
	Lookup         string
	CreatedAt      time.Time
	LastUsedAt     runtime.Null[time.Time]
	ExpiresAt      time.Time
	RevokedAt      runtime.Null[time.Time]
}

// ListPlatformAPIKeys lists every key with its owner's username resolved.
var ListPlatformAPIKeys = storm.SQL[APIKeyListRow](`
SELECT k.id::text AS id, k.platform_user_id::text AS platform_user_id, u.username,
       k.label, k.lookup, k.created_at, k.last_used_at, k.expires_at, k.revoked_at
FROM platform_api_keys k
JOIN platform_users u ON u.id = k.platform_user_id
ORDER BY k.created_at DESC`)

// TouchPlatformAPIKeyUsed records a use. Server clock, so "last used" is
// comparable across app servers.
var TouchPlatformAPIKeyUsed = storm.SQLExec(`
UPDATE platform_api_keys SET last_used_at = now() WHERE id = $1`)

// RevokePlatformAPIKey revokes a live key and reports whether it did.
//
// The revoked_at IS NULL guard is what makes the row count meaningful: without
// it a second revocation also reports one row, and the caller cannot tell "I
// revoked it" from "somebody already had".
var RevokePlatformAPIKey = storm.SQLExec(`
UPDATE platform_api_keys SET revoked_at = now()
WHERE id = $1 AND revoked_at IS NULL`)
