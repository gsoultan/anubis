-- name: CreateSession :one
INSERT INTO sessions (identity_id, tenant_id, application_id, amr, device_fp,
                      ip, user_agent, active_scopes, expires_at)
VALUES (sqlc.arg(identity_id), sqlc.arg(tenant_id), sqlc.narg(application_id),
        sqlc.arg(amr)::text[], nullif(sqlc.arg(device_fp), ''),
        nullif(sqlc.arg(ip), '')::inet, nullif(sqlc.arg(user_agent), ''),
        sqlc.arg(active_scopes)::jsonb, sqlc.arg(expires_at))
RETURNING id, created_at, auth_time, expires_at;

-- name: GetSessionLive :one
SELECT s.id, s.identity_id, s.tenant_id, s.application_id, s.amr,
       s.created_at, s.last_seen_at, s.auth_time, s.expires_at,
       s.active_scopes, s.device_fp,
       i.token_epoch, i.status AS identity_status,
       i.assurance_level, i.username, i.email, i.realm_id,
       r.code AS realm_code
FROM sessions s
JOIN identities i ON i.id = s.identity_id AND i.tenant_id = s.tenant_id
LEFT JOIN realms r ON r.id = i.realm_id
WHERE s.id = sqlc.arg(id)
  AND s.revoked_at IS NULL AND s.expires_at > now();

-- name: GetSessionState :one
-- Introspection: is this sid still good, and what epoch does the identity hold.
SELECT s.revoked_at, s.expires_at, i.token_epoch, i.status AS identity_status,
       i.disabled_at, i.anonymized_at
FROM sessions s
JOIN identities i ON i.id = s.identity_id AND i.tenant_id = s.tenant_id
WHERE s.id = sqlc.arg(id) AND s.tenant_id = sqlc.arg(tenant_id);

-- name: ListSessionsByIdentity :many
SELECT id, created_at, last_seen_at, expires_at, amr,
       COALESCE(host(ip)::text, '')::text AS ip, user_agent
FROM sessions
WHERE identity_id = sqlc.arg(identity_id) AND revoked_at IS NULL
  AND expires_at > now()
ORDER BY created_at DESC;

-- name: RevokeSession :one
UPDATE sessions
SET revoked_at = now(), revoke_reason = sqlc.arg(reason), cookie_hash = NULL
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id) AND revoked_at IS NULL
RETURNING id, identity_id, application_id;

-- name: RevokeAllSessions :many
UPDATE sessions
SET revoked_at = now(), revoke_reason = sqlc.arg(reason), cookie_hash = NULL
WHERE identity_id = sqlc.arg(identity_id) AND tenant_id = sqlc.arg(tenant_id)
  AND revoked_at IS NULL
RETURNING id, application_id;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now() WHERE id = sqlc.arg(id);

-- name: UpdateSessionScopes :exec
UPDATE sessions SET active_scopes = sqlc.arg(active_scopes)::jsonb
WHERE id = sqlc.arg(id);

-- name: UpgradeSessionAmr :one
-- Step-up: MFA verified mid-session; auth_time restarts the recency window.
UPDATE sessions SET amr = sqlc.arg(amr)::text[], auth_time = now()
WHERE id = sqlc.arg(id) AND revoked_at IS NULL
RETURNING auth_time;

-- name: SetSessionCookieHash :exec
UPDATE sessions SET cookie_hash = sqlc.arg(cookie_hash) WHERE id = sqlc.arg(id);

-- name: GetSessionByCookieHash :one
SELECT s.id, s.identity_id, s.tenant_id, s.amr, s.auth_time, s.expires_at,
       s.active_scopes, i.username, i.realm_id, r.code AS realm_code
FROM sessions s
JOIN identities i ON i.id = s.identity_id AND i.tenant_id = s.tenant_id
LEFT JOIN realms r ON r.id = i.realm_id
WHERE s.cookie_hash = sqlc.arg(cookie_hash)
  AND s.revoked_at IS NULL AND s.expires_at > now();

-- name: RecentlyRevokedSessions :many
-- Gate snapshot denylist. Bounded by the longest access-token TTL: a
-- revocation older than that cannot match a still-valid token.
SELECT id FROM sessions
WHERE tenant_id = sqlc.arg(tenant_id)
  AND revoked_at IS NOT NULL
  AND revoked_at > now() - sqlc.arg(revoked_window)::text::interval;
