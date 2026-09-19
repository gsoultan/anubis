package identityrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// PasswordRow is the one live password credential an identity may hold.
type PasswordRow struct {
	ID         string
	IdentityID string
	TenantID   string
	Secret     runtime.Null[string]
	Params     runtime.JSON
	ExpiresAt  runtime.Null[time.Time]
}

var GetPasswordCredential = storm.SQL[PasswordRow](`
SELECT id::text AS id, identity_id::text AS identity_id,
       tenant_id::text AS tenant_id, secret, params, expires_at
FROM credentials
WHERE identity_id = $1 AND kind = 'password' AND revoked_at IS NULL`)

// CreatedCredentialRow is a new credential's id and clock.
type CreatedCredentialRow struct {
	ID        string
	CreatedAt time.Time
}

// CreateCredential writes one way of proving an identity.
//
// nullif on three columns: a credential with no secret (a device key), no
// lookup key, or no label is a real shape, and the partial unique index on
// lookup_key expects NULL rather than ” — every keyless credential would
// otherwise collide with every other.
var CreateCredential = storm.SQL[CreatedCredentialRow](`
INSERT INTO credentials (identity_id, tenant_id, kind, secret, lookup_key,
                         label, params, expires_at)
VALUES ($1, $2, $3, nullif($4, ''), nullif($5, ''), nullif($6, ''), $7::jsonb, $8)
RETURNING id::text AS id, created_at`)

var RevokeCredential = storm.SQLExec(`
UPDATE credentials SET revoked_at = now(), updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL`)

var RevokeCredentialsOfKind = storm.SQLExec(`
UPDATE credentials SET revoked_at = now(), updated_at = now()
WHERE identity_id = $1 AND kind = $2 AND revoked_at IS NULL`)

// TouchCredentialUsed records a use and advances the WebAuthn replay guard.
//
// GREATEST, not assignment: the counter only ever increases, and an
// authenticator that reports a LOWER value than we have seen is the signal
// that its credential has been cloned. Writing the reported value would erase
// that evidence.
var TouchCredentialUsed = storm.SQLExec(`
UPDATE credentials
SET last_used_at = now(),
    sign_counter = GREATEST(sign_counter, $2),
    updated_at = now()
WHERE id = $1`)

// UpdateCredentialSecret is the KDF upgrade path: rehash on next successful
// login (ADR-0002).
var UpdateCredentialSecret = storm.SQLExec(`
UPDATE credentials SET secret = $2, updated_at = now() WHERE id = $1`)

// UpdateCredentialParams carries the TOTP replay guard: params holds the last
// accepted time step.
var UpdateCredentialParams = storm.SQLExec(`
UPDATE credentials SET params = $2::jsonb, updated_at = now() WHERE id = $1`)

// KindRow is one credential kind an identity currently holds.
type KindRow struct {
	Kind string
}

// ListActiveCredentialKinds is what the sign-in screen offers.
//
// DISTINCT, and expiry-aware: an expired recovery-code set must not advertise
// a factor the person can no longer produce.
var ListActiveCredentialKinds = storm.SQL[KindRow](`
SELECT DISTINCT kind FROM credentials
WHERE identity_id = $1 AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > now())`)

// ActiveCredentialRow is the newest live credential of one kind.
type ActiveCredentialRow struct {
	ID          string
	IdentityID  string
	TenantID    string
	Kind        string
	Secret      runtime.Null[string]
	Params      runtime.JSON
	SignCounter int64
}

var GetActiveCredentialOfKind = storm.SQL[ActiveCredentialRow](`
SELECT id::text AS id, identity_id::text AS identity_id,
       tenant_id::text AS tenant_id, kind, secret, params, sign_counter
FROM credentials
WHERE identity_id = $1 AND kind = $2
  AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at DESC
LIMIT 1`)

// IDRow is one row's id.
type IDRow struct {
	ID string
}

// ConsumeRecoveryCode is single use: the row is revoked AS it is accepted, in
// one statement, so two concurrent presentations cannot both win.
//
// The subselect picks the row and the UPDATE claims it; a read-then-revoke
// would let both callers read the same live code.
var ConsumeRecoveryCode = storm.SQL[IDRow](`
UPDATE credentials
SET revoked_at = now(), last_used_at = now(), updated_at = now()
WHERE id = (
    SELECT c.id FROM credentials c
     WHERE c.identity_id = $1
       AND c.kind = 'recovery_code'
       AND c.revoked_at IS NULL
       AND c.secret = $2
     LIMIT 1)
RETURNING id::text AS id`)
