package authrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// IDRow is a freshly written row's id.
type IDRow struct {
	ID string
}

// CreateRefreshToken writes one generation of a family.
//
// nullif on bound_key: ” is not a binding, and a token bound to the empty
// string would match a presenter who sent no key at all.
//
// client_id is the application the family was issued to (0051), written on
// every generation so a rotation can re-issue for it; empty means none.
var CreateRefreshToken = storm.SQL[IDRow](`
INSERT INTO refresh_tokens (session_id, tenant_id, family_id, generation,
                            token_hash, expires_at, bound_key, client_id)
VALUES ($1, $2, $3, $4, $5, $6, nullif($7, ''), nullif($8, ''))
RETURNING id::text AS id`)

// ClaimedRefreshRow is the token a caller just won the right to rotate.
type ClaimedRefreshRow struct {
	ID         string
	SessionID  string
	TenantID   string
	FamilyID   string
	Generation int32
	ExpiresAt  time.Time
	ClientID   string
}

// ClaimRefreshToken is the rotation core.
//
// Exactly one caller can flip active -> consumed. A second presentation of the
// same token changes no rows, falls through to GetRefreshTokenByHash, and is
// recognised as THEFT — which is the whole design: the guard is in the
// statement because a read-then-write would let both presentations pass the
// read and both be issued a successor.
//
// bound_key IS NULL: a token bound to a key (0002 reserved it for DPoP-style
// binding) must not rotate for somebody who cannot prove the key, and this
// path cannot check a proof yet. Nothing binds a token today; this stops a
// half-built binding from being quietly skipped here later.
var ClaimRefreshToken = storm.SQL[ClaimedRefreshRow](`
UPDATE refresh_tokens
SET status = 'consumed', consumed_at = now()
WHERE token_hash = $1
  AND status = 'active'
  AND expires_at > now()
  AND bound_key IS NULL
RETURNING id::text AS id, session_id::text AS session_id,
          tenant_id::text AS tenant_id, family_id::text AS family_id,
          generation, expires_at, coalesce(client_id, '') AS client_id`)

// SetRefreshSuccessor links a consumed token to the one that replaced it.
//
// expires_at is in the WHERE because the table is partitioned on it: without
// it the UPDATE would have to scan every partition to find one row.
var SetRefreshSuccessor = storm.SQLExec(`
UPDATE refresh_tokens SET successor_id = $2
WHERE id = $1 AND expires_at = $3`)

// RefreshTokenRow is a token looked up by hash, whatever state it is in —
// which is the point: a consumed or revoked one is what theft looks like.
type RefreshTokenRow struct {
	ID         string
	SessionID  string
	TenantID   string
	FamilyID   string
	Generation int32
	Status     string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	ConsumedAt runtime.Null[time.Time]
	RevokedAt  runtime.Null[time.Time]
}

var GetRefreshTokenByHash = storm.SQL[RefreshTokenRow](`
SELECT id::text AS id, session_id::text AS session_id,
       tenant_id::text AS tenant_id, family_id::text AS family_id,
       generation, status, created_at, expires_at, consumed_at, revoked_at
FROM refresh_tokens
WHERE token_hash = $1`)

// RevokeRefreshFamily is the theft response: the whole family dies, whatever
// generation the attacker holds.
var RevokeRefreshFamily = storm.SQLExec(`
UPDATE refresh_tokens
SET status = 'revoked', revoked_at = now()
WHERE family_id = $1 AND status = 'active'`)

var RevokeRefreshBySession = storm.SQLExec(`
UPDATE refresh_tokens
SET status = 'revoked', revoked_at = now()
WHERE session_id = $1 AND status = 'active'`)

// RevokeRefreshBySessions takes a list, so signing out everywhere is one
// statement rather than one per session.
var RevokeRefreshBySessions = storm.SQLExec(`
UPDATE refresh_tokens
SET status = 'revoked', revoked_at = now()
WHERE session_id = ANY($1::uuid[]) AND status = 'active'`)
