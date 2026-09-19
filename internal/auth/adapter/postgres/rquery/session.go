package authrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// CreatedSessionRow is a fresh session's identity and clocks.
type CreatedSessionRow struct {
	ID        string
	CreatedAt time.Time
	AuthTime  time.Time
	ExpiresAt time.Time
}

// CreateSession opens a sign-in.
//
// nullif on three columns: an absent device fingerprint, IP or user agent is
// NULL, not ”. The distinction is not cosmetic — a session listing that shows
// an empty string where it means "we were not told" invites somebody to read
// it as a device that reported nothing.
var CreateSession = storm.SQL[CreatedSessionRow](`
INSERT INTO sessions (identity_id, tenant_id, application_id, amr, device_fp,
                      ip, user_agent, active_scopes, expires_at)
VALUES ($1, $2, $3, $4::text[], nullif($5, ''), nullif($6, '')::inet,
        nullif($7, ''), $8::jsonb, $9)
RETURNING id::text AS id, created_at, auth_time, expires_at`)

// LiveSessionRow is a session the request path resolves, with the identity
// facts it must re-check on every call.
type LiveSessionRow struct {
	ID             string
	IdentityID     string
	TenantID       string
	ApplicationID  runtime.Null[string]
	Amr            []string
	CreatedAt      time.Time
	LastSeenAt     time.Time
	AuthTime       time.Time
	ExpiresAt      time.Time
	ActiveScopes   runtime.JSON
	DeviceFp       runtime.Null[string]
	TokenEpoch     int32
	IdentityStatus string
	AssuranceLevel int16
	Username       runtime.Null[string]
	Email          runtime.Null[string]
	RealmID        runtime.Null[string]
	RealmCode      runtime.Null[string]
}

// GetSessionLive resolves a session AND the identity behind it in one query.
//
// The identity's epoch and status are read here rather than trusted from when
// the session opened: disabling somebody has to take their live sessions down
// with them, and a second query would be a window in which it did not.
//
// LEFT JOIN on realms because an identity need not belong to one.
var GetSessionLive = storm.SQL[LiveSessionRow](`
SELECT s.id::text AS id, s.identity_id::text AS identity_id,
       s.tenant_id::text AS tenant_id, s.application_id::text AS application_id,
       s.amr, s.created_at, s.last_seen_at, s.auth_time, s.expires_at,
       s.active_scopes, s.device_fp,
       i.token_epoch, i.status AS identity_status,
       i.assurance_level, i.username, i.email, i.realm_id::text AS realm_id,
       r.code AS realm_code
FROM sessions s
JOIN identities i ON i.id = s.identity_id AND i.tenant_id = s.tenant_id
LEFT JOIN realms r ON r.id = i.realm_id
WHERE s.id = $1
  AND s.revoked_at IS NULL AND s.expires_at > now()`)

// SessionStateRow is what introspection needs and nothing more.
type SessionStateRow struct {
	RevokedAt      runtime.Null[time.Time]
	ExpiresAt      time.Time
	TokenEpoch     int32
	IdentityStatus string
	DisabledAt     runtime.Null[time.Time]
	AnonymizedAt   runtime.Null[time.Time]
}

// GetSessionState answers "is this sid still good, and what epoch does the
// identity hold" — the whole of what a token introspection call needs.
var GetSessionState = storm.SQL[SessionStateRow](`
SELECT s.revoked_at, s.expires_at, i.token_epoch, i.status AS identity_status,
       i.disabled_at, i.anonymized_at
FROM sessions s
JOIN identities i ON i.id = s.identity_id AND i.tenant_id = s.tenant_id
WHERE s.id = $1 AND s.tenant_id = $2`)

// SessionListRow is one of a person's own sessions, as they see it.
type SessionListRow struct {
	ID         string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	Amr        []string
	IP         string
	UserAgent  runtime.Null[string]
}

// ListSessionsByIdentity is the "where am I signed in" screen.
//
// host(ip) drops the mask an inet carries: /32 on every row is noise to
// somebody deciding whether they recognise a login.
var ListSessionsByIdentity = storm.SQL[SessionListRow](`
SELECT id::text AS id, created_at, last_seen_at, expires_at, amr,
       COALESCE(host(ip)::text, '')::text AS ip, user_agent
FROM sessions
WHERE identity_id = $1 AND revoked_at IS NULL AND expires_at > now()
ORDER BY created_at DESC`)

// RevokedSessionRow identifies a session that has just been revoked, so the
// caller can tell its application.
type RevokedSessionRow struct {
	ID            string
	IdentityID    string
	ApplicationID runtime.Null[string]
}

// RevokeSession ends one session and clears its cookie hash in the same
// statement, so the cookie stops resolving immediately rather than at expiry.
//
// The revoked_at IS NULL guard makes the returned row meaningful: without it a
// second revocation also returns a row, and the caller cannot tell "I revoked
// it" from "somebody already had".
var RevokeSession = storm.SQL[RevokedSessionRow](`
UPDATE sessions
SET revoked_at = now(), revoke_reason = $3, cookie_hash = NULL
WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL
RETURNING id::text AS id, identity_id::text AS identity_id,
          application_id::text AS application_id`)

// RevokedAllRow is one session ended by a sign-out-everywhere.
type RevokedAllRow struct {
	ID            string
	ApplicationID runtime.Null[string]
}

// RevokeAllSessions is sign-out-everywhere; it returns each session so every
// application can be told.
var RevokeAllSessions = storm.SQL[RevokedAllRow](`
UPDATE sessions
SET revoked_at = now(), revoke_reason = $3, cookie_hash = NULL
WHERE identity_id = $1 AND tenant_id = $2 AND revoked_at IS NULL
RETURNING id::text AS id, application_id::text AS application_id`)

var UpdateSessionScopes = storm.SQLExec(`
UPDATE sessions SET active_scopes = $2::jsonb WHERE id = $1`)

// AuthTimeRow is the restarted recency window.
type AuthTimeRow struct {
	AuthTime time.Time
}

// UpgradeSessionAmr is the step-up: MFA verified mid-session.
//
// auth_time restarts, which is what a max_age check reads — a step-up that
// recorded the method without moving the clock would leave the session still
// too old for the operation that demanded the step-up.
var UpgradeSessionAmr = storm.SQL[AuthTimeRow](`
UPDATE sessions SET amr = $2::text[], auth_time = now()
WHERE id = $1 AND revoked_at IS NULL
RETURNING auth_time`)

var SetSessionCookieHash = storm.SQLExec(`
UPDATE sessions SET cookie_hash = $2 WHERE id = $1`)

// CookieSessionRow is a session resolved from a browser cookie.
type CookieSessionRow struct {
	ID           string
	IdentityID   string
	TenantID     string
	Amr          []string
	AuthTime     time.Time
	ExpiresAt    time.Time
	ActiveScopes runtime.JSON
	Username     runtime.Null[string]
	RealmID      runtime.Null[string]
	RealmCode    runtime.Null[string]
}

var GetSessionByCookieHash = storm.SQL[CookieSessionRow](`
SELECT s.id::text AS id, s.identity_id::text AS identity_id,
       s.tenant_id::text AS tenant_id, s.amr, s.auth_time, s.expires_at,
       s.active_scopes, i.username, i.realm_id::text AS realm_id, r.code AS realm_code
FROM sessions s
JOIN identities i ON i.id = s.identity_id AND i.tenant_id = s.tenant_id
LEFT JOIN realms r ON r.id = i.realm_id
WHERE s.cookie_hash = $1
  AND s.revoked_at IS NULL AND s.expires_at > now()`)
