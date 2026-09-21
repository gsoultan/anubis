package controlrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// PlatformUserListRow is one operator as the console lists them. No password
// hash and no totp secret: a listing screen needs neither.
type PlatformUserListRow struct {
	ID             string
	Username       string
	Email          runtime.Null[string]
	Status         string
	TokenEpoch     int32
	LastLoginAt    runtime.Null[time.Time]
	DisabledAt     runtime.Null[time.Time]
	CreatedAt      time.Time
	TotpEnrolledAt runtime.Null[time.Time]
}

// ListPlatformUsers is keyset-paginated on username.
//
// OFFSET over a growing table re-scans everything it skips and can show a row
// twice when one is inserted mid-page; a keyset cannot. It stays SQL because
// the search is an ILIKE with a wildcard the caller does not supply — building
// '%' || $1 || '%' is a concatenation, not a value, and doing it in Go would
// put a pattern the user typed into a LIKE without anybody reading it as one.
var ListPlatformUsers = storm.SQL[PlatformUserListRow](`
SELECT id::text AS id, username, email, status, token_epoch, last_login_at,
       disabled_at, created_at, totp_enrolled_at
  FROM platform_users
 WHERE ($1::text = '' OR username ILIKE '%' || $1::text || '%')
   AND ($2::text = '' OR username > $2::text)
 ORDER BY username
 LIMIT $3`)

// SetPlatformUserStatus disables or re-enables an operator.
//
// The three columns move together for a reason. token_epoch + 1 is what makes
// disabling take effect NOW rather than whenever the token expired, and it is
// computed in the database because in Go it is a read-modify-write: two
// concurrent disables would both read N, both write N+1, and one operator's
// tokens would stay live.
var SetPlatformUserStatus = storm.SQLExec(`
UPDATE platform_users
   SET status = $2, updated_at = now(),
       disabled_at = CASE WHEN $2::text = 'disabled' THEN now() ELSE NULL END,
       token_epoch = token_epoch + CASE WHEN $2::text = 'disabled' THEN 1 ELSE 0 END
 WHERE id = $1`)

// RehashPlatformUserPassword upgrades a hash whose KDF parameters are behind
// the current default.
//
// It is guarded on the OLD hash rather than on the id alone. A rehash races
// every other write to the row, and a bare `SET password_hash = $2` would
// happily overwrite a password that changed between the read and this write —
// putting back the one the operator just replaced.
var RehashPlatformUserPassword = storm.SQLExec(`
UPDATE platform_users
   SET password_hash = $3, updated_at = now()
 WHERE id = $1 AND password_hash = $2`)

// SetPlatformUserPassword replaces the password and supersedes every token
// minted under the old one.
//
// The two columns move together for the same reason SetPlatformUserStatus
// moves three: token_epoch + 1 is what ends the sessions, and computing it in
// the database keeps two concurrent changes from both reading N and both
// writing N+1 — which would leave one of them believing it had cut off
// tokens it had not.
var SetPlatformUserPassword = storm.SQLExec(`
UPDATE platform_users
   SET password_hash = $2, token_epoch = token_epoch + 1, updated_at = now()
 WHERE id = $1`)

// TouchPlatformUserLogin records a sign-in against the server clock.
var TouchPlatformUserLogin = storm.SQLExec(`
UPDATE platform_users SET last_login_at = now() WHERE id = $1`)

// StageTotpSecret stores a secret that has NOT been confirmed yet.
//
// Enrolment is not complete until a code verifies, so this deliberately leaves
// totp_enrolled_at alone: holding a secret must never start demanding a factor
// the operator cannot yet produce.
var StageTotpSecret = storm.SQLExec(`
UPDATE platform_users
   SET totp_secret_enc = $2, updated_at = now()
 WHERE id = $1`)

// ConfirmTotpEnrolment completes enrolment, and only when a secret is staged —
// otherwise a caller could mark an account as carrying a factor it has none of.
var ConfirmTotpEnrolment = storm.SQLExec(`
UPDATE platform_users
   SET totp_enrolled_at = now(), totp_last_step = $2, updated_at = now()
 WHERE id = $1 AND totp_secret_enc IS NOT NULL`)

// AdvanceTotpStep is the single-use guard: it only succeeds when the step is
// strictly newer than the last one accepted, so replaying a code inside its own
// validity window updates nothing and the caller refuses the login.
//
// The comparison is in the WHERE, not in Go, deliberately: read-then-write
// would let two presentations of the same code both pass the read.
var AdvanceTotpStep = storm.SQLExec(`
UPDATE platform_users
   SET totp_last_step = $2, updated_at = now()
 WHERE id = $1 AND totp_last_step < $2`)

// ClearTotp removes the factor entirely, resetting the step so a later
// enrolment does not inherit a high-water mark from the old secret.
var ClearTotp = storm.SQLExec(`
UPDATE platform_users
   SET totp_secret_enc = NULL, totp_enrolled_at = NULL, totp_last_step = 0,
       updated_at = now()
 WHERE id = $1`)
