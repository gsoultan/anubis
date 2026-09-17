package controlrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// IDRow is a freshly written row's id.
type IDRow struct {
	ID string
}

// InsertPlatformRefreshRoot begins a family: the row's own id IS the family
// id, which is what lets one revocation kill a sign-in however many times it
// has rotated.
//
// The uuid is generated once in a subquery and used twice. Generating it in Go
// would work; generating it twice in SQL would not, and the shape that makes
// that impossible is worth more than the line it costs.
var InsertPlatformRefreshRoot = storm.SQL[IDRow](`
INSERT INTO platform_refresh_tokens (id, platform_user_id, family_id, token_hash, expires_at)
SELECT g.u, $1, g.u, $2, $3 FROM (SELECT uuidv7() AS u) g
RETURNING id::text AS id`)

// RefreshRow is one refresh token, hash excluded — the caller already has it.
type RefreshRow struct {
	ID             string
	PlatformUserID string
	FamilyID       string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	UsedAt         runtime.Null[time.Time]
	RevokedAt      runtime.Null[time.Time]
}

// PlatformRefreshByHash resolves a presented token.
//
// SQL rather than a builder predicate because storm has no comparison on a
// bytea column (codegen/tree.go: "bytea offers no predicates at all"), and
// this table is looked up by hash and nothing else — its unique index IS
// token_hash.
var PlatformRefreshByHash = storm.SQL[RefreshRow](`
SELECT id::text AS id, platform_user_id::text AS platform_user_id,
       family_id::text AS family_id, created_at, expires_at, used_at, revoked_at
FROM platform_refresh_tokens
WHERE token_hash = $1`)

// ConsumePlatformRefresh is the rotation's atomic heart: exactly one caller can
// flip used_at, and a second concurrent presenter gets zero rows — which the
// interactor treats as reuse, not as a race to shrug at.
var ConsumePlatformRefresh = storm.SQLExec(`
UPDATE platform_refresh_tokens
SET used_at = now()
WHERE id = $1 AND used_at IS NULL AND revoked_at IS NULL`)

// RevokePlatformRefreshFamily kills every token descended from one sign-in.
var RevokePlatformRefreshFamily = storm.SQLExec(`
UPDATE platform_refresh_tokens
SET revoked_at = now()
WHERE family_id = $1 AND revoked_at IS NULL`)

// SweepPlatformRefresh deletes tokens a day past expiry.
//
// A day, not zero: a just-expired row still answers "was this reused?" for the
// window in which somebody might present it.
var SweepPlatformRefresh = storm.SQLExec(`
DELETE FROM platform_refresh_tokens
WHERE expires_at < now() - interval '1 day'`)
