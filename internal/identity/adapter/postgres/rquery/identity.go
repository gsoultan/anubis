package identityrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// LoginRow is the login lookup's answer, with the realm policy riding along.
type LoginRow struct {
	ID             string
	TenantID       string
	TokenEpoch     int32
	Status         string
	Username       string
	Email          runtime.Null[string]
	AssuranceLevel int16
	DisabledAt     runtime.Null[time.Time]
	AnonymizedAt   runtime.Null[time.Time]
	RealmID        runtime.Null[string]
	RealmCode      runtime.Null[string]
	RealmKind      runtime.Null[string]
}

// GetIdentityForLogin matches on lower(username), which is the expression the
// unique index is built on. storm can say that now — EqLower — so the
// predicate is not what keeps this in SQL.
//
// The LEFT JOIN is: an identity predating realm assignment still has to
// resolve, and the realm's policy rides along to avoid a second round trip on
// the hot path. storm's builder reads one table.
var GetIdentityForLogin = storm.SQL[LoginRow](`
SELECT i.id::text AS id, i.tenant_id::text AS tenant_id, i.token_epoch, i.status,
       i.username, i.email, i.assurance_level, i.disabled_at, i.anonymized_at,
       i.realm_id::text AS realm_id, r.code AS realm_code, r.kind AS realm_kind
FROM identities i
LEFT JOIN realms r ON r.id = i.realm_id
WHERE i.tenant_id = $1
  AND i.realm_id = $2
  AND lower(i.username) = lower($3)`)

// IdentityRow is one identity as the console shows it.
type IdentityRow struct {
	ID             string
	TenantID       string
	TokenEpoch     int32
	Status         string
	Username       string
	Email          runtime.Null[string]
	ExternalRef    runtime.Null[string]
	AssuranceLevel int16
	DisabledAt     runtime.Null[time.Time]
	AnonymizedAt   runtime.Null[time.Time]
	CreatedAt      time.Time
	LastLoginAt    runtime.Null[time.Time]
	RetentionUntil runtime.Null[time.Time]
	RealmID        runtime.Null[string]
	CategoryID     runtime.Null[string]
	RealmCode      runtime.Null[string]
	RealmKind      runtime.Null[string]
	CategoryCode   runtime.Null[string]
}

const identityCols = `
       i.id::text AS id, i.tenant_id::text AS tenant_id, i.token_epoch, i.status,
       i.username, i.email, i.external_ref, i.assurance_level,
       i.disabled_at, i.anonymized_at, i.created_at, i.last_login_at,
       i.retention_until, i.realm_id::text AS realm_id,
       i.category_id::text AS category_id,
       r.code AS realm_code, r.kind AS realm_kind, c.code AS category_code`

const identityJoins = `
FROM identities i
LEFT JOIN realms r ON r.id = i.realm_id
LEFT JOIN realm_categories c ON c.id = i.category_id`

var GetIdentity = storm.SQL[IdentityRow](`
SELECT` + identityCols + identityJoins + `
WHERE i.id = $1 AND i.tenant_id = $2`)

// ListIdentities is KEYSET paginated on id.
//
// uuidv7 is time-ordered, so id order IS creation order and the probe stays an
// index range scan at any offset — which OFFSET would not, and which is what
// makes the last page of a 57,000-row tenant cost the same as the first.
var ListIdentities = storm.SQL[IdentityRow](`
SELECT` + identityCols + identityJoins + `
WHERE i.tenant_id = $1
  AND ($2::uuid IS NULL OR i.realm_id = $2)
  AND ($3::text IS NULL OR i.status = $3)
  AND ($4::text IS NULL
       OR i.username ILIKE '%' || $4 || '%'
       OR i.email    ILIKE '%' || $4 || '%')
  AND ($5::uuid IS NULL OR i.id > $5)
ORDER BY i.id
LIMIT $6`)

// CreatedIdentityRow is a new identity's id and starting epoch.
type CreatedIdentityRow struct {
	ID         string
	CreatedAt  time.Time
	TokenEpoch int32
}

// CreateIdentity writes one person.
//
// nullif on email and external_ref: ” is not an address and not an upstream
// id, and the partial unique indexes on both expect NULL for absence — an
// empty string would make every unlinked identity collide with every other.
var CreateIdentity = storm.SQL[CreatedIdentityRow](`
INSERT INTO identities (tenant_id, realm_id, username, email, external_ref,
                        assurance_level, category_id, status)
VALUES ($1, $2, $3, nullif($4, ''), nullif($5, ''), $6, $7, $8)
RETURNING id::text AS id, created_at, token_epoch`)

// DisableIdentity is guarded on disabled_at so the row count distinguishes
// "I disabled it" from "somebody already had".
var DisableIdentity = storm.SQLExec(`
UPDATE identities SET status = 'disabled', disabled_at = now(), updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND disabled_at IS NULL`)

var EnableIdentity = storm.SQLExec(`
UPDATE identities SET status = 'active', disabled_at = NULL, updated_at = now()
WHERE id = $1 AND tenant_id = $2`)

// TokenEpochRow is the epoch after a bump.
type TokenEpochRow struct {
	TokenEpoch int32
}

// BumpTokenEpoch invalidates every outstanding token for one identity.
//
// token_epoch + 1 is computed by the DATABASE: in Go it is a read-modify-write,
// and two concurrent revocations would both read N, both write N+1, and leave
// one set of tokens live.
var BumpTokenEpoch = storm.SQL[TokenEpochRow](`
UPDATE identities SET token_epoch = token_epoch + 1, updated_at = now()
WHERE id = $1 AND tenant_id = $2
RETURNING token_epoch`)

var TouchLastLogin = storm.SQLExec(`
UPDATE identities SET last_login_at = now() WHERE id = $1`)

// RequestErasure records the request; the erasure itself is AnonymizeIdentity.
var RequestErasure = storm.SQLExec(`
UPDATE identities SET deletion_requested_at = now(), updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deletion_requested_at IS NULL`)

var LinkIdentities = storm.SQLExec(`
INSERT INTO identity_links (tenant_id, primary_id, secondary_id, linked_by, method, evidence)
VALUES ($1, $2, $3, $4, $5, $6::jsonb)`)

// AuthStateRow is the two identity facts that outrank any token claim.
type AuthStateRow struct {
	TokenEpoch   int32
	Status       string
	DisabledAt   runtime.Null[time.Time]
	AnonymizedAt runtime.Null[time.Time]
}

var GetIdentityAuthState = storm.SQL[AuthStateRow](`
SELECT token_epoch, status, disabled_at, anonymized_at
FROM identities
WHERE id = $1 AND tenant_id = $2`)

// ErasedRow identifies an identity that was just anonymised, and the PII key
// that now needs shredding.
type ErasedRow struct {
	ID       string
	TenantID string
	PiiKeyID runtime.Null[string]
}

// The anonymisation both sweeps and erasures perform. It is one statement
// because it has to be atomic: blanking the identifiers without bumping the
// epoch leaves live tokens for a person who no longer exists.
const anonymiseSet = `
SET anonymized_at = now(),
    status        = 'disabled',
    username      = 'anon_' || left(replace(id::text, '-', ''), 16),
    email         = NULL,
    external_ref  = NULL,
    attributes    = '{}'::jsonb,
    token_epoch   = token_epoch + 1,
    updated_at    = now()`

// ExpireRetainedIdentities is the retention sweep: identities past their
// statutory limit are ANONYMISED, not deleted — rows and referential integrity
// survive for audit while authorize() denies from that moment
// (migrations/0009 gate 1).
var ExpireRetainedIdentities = storm.SQL[ErasedRow](`
UPDATE identities` + anonymiseSet + `
WHERE retention_until IS NOT NULL
  AND retention_until < now()
  AND anonymized_at IS NULL
RETURNING id::text AS id, tenant_id::text AS tenant_id, pii_key_id::text AS pii_key_id`)

// AnonymizeIdentity is right-to-erasure execution. It additionally clears
// pii_key_id, because the key is crypto-shredded separately and a dangling
// reference would read as corruption rather than as a completed erasure.
var AnonymizeIdentity = storm.SQL[ErasedRow](`
UPDATE identities` + anonymiseSet + `,
    pii_key_id    = NULL
WHERE id = $1 AND tenant_id = $2
  AND anonymized_at IS NULL
RETURNING id::text AS id, tenant_id::text AS tenant_id, pii_key_id::text AS pii_key_id`)

// SetRetentionFromRealm applies a realm's default_retention to the identities
// that have none yet — not to those that do, so an operator's explicit date
// is never overwritten by a policy change.
var SetRetentionFromRealm = storm.SQLExec(`
UPDATE identities i
SET retention_until = now() + r.default_retention, updated_at = now()
FROM realms r
WHERE i.realm_id = r.id
  AND r.default_retention IS NOT NULL
  AND i.retention_until IS NULL
  AND i.anonymized_at IS NULL`)

// RealmCountRow is one population's size.
type RealmCountRow struct {
	Realm string
	Kind  string
	N     int64
}

// CountIdentitiesByRealm LEFT JOINs from realms, so a realm with nobody in it
// still appears with a zero — an inner join would hide exactly the populations
// somebody is about to investigate.
var CountIdentitiesByRealm = storm.SQL[RealmCountRow](`
SELECT r.code AS realm, r.kind, count(i.id) AS n
FROM realms r
LEFT JOIN identities i ON i.realm_id = r.id AND i.tenant_id = r.tenant_id
WHERE r.tenant_id = $1
GROUP BY r.code, r.kind
ORDER BY r.code`)

// CountRow is a single count.
type CountRow struct {
	N int64
}

// CountRetentionBacklog is rows past their retention deadline and not yet
// anonymised — each one a compliance clock already ringing.
var CountRetentionBacklog = storm.SQL[CountRow](`
SELECT count(*) AS n FROM identities
WHERE tenant_id = $1 AND retention_until IS NOT NULL
  AND retention_until < now() AND anonymized_at IS NULL`)

// CategoryCountRow is how many people sit in one category.
type CategoryCountRow struct {
	CategoryID string
	N          int64
}

// CountIdentitiesByCategory is counted in the DATABASE because the console
// used to count rows it had fetched — capped at 2,000 of 57,000, so every
// figure was wrong.
//
// realm_id nullable, matching ListRealmCategories: a tenant-wide listing has
// no realm to count within, and the two have to agree or the counts belong to
// a different set of categories than the names.
var CountIdentitiesByCategory = storm.SQL[CategoryCountRow](`
SELECT category_id::text AS category_id, count(*) AS n
FROM identities
WHERE tenant_id = $1
  AND ($2::uuid IS NULL OR realm_id = $2)
  AND category_id IS NOT NULL
GROUP BY category_id`)
