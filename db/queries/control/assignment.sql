-- Control-plane authority (ADR-0011). These rows say which tenants a platform
-- user may administer; they are never a substitute for a grant, which is what
-- a tenant's own members hold.

-- name: CreatePlatformAssignment :one
INSERT INTO platform_assignments (operator_id, tenant_id, role, granted_by, reason, valid_until)
VALUES (sqlc.arg(operator_id), sqlc.narg(tenant_id), sqlc.arg(role),
        sqlc.narg(granted_by), sqlc.arg(reason), sqlc.narg(valid_until))
RETURNING id;

-- ListAssignments is the user-management page's read: every live assignment,
-- with the tenant slug resolved for display.
-- name: ListAssignments :many
SELECT a.id, a.operator_id, a.tenant_id, t.slug AS tenant_slug, a.role,
       a.granted_by, a.reason, a.valid_until, a.revoked_at, a.created_at
  FROM platform_assignments a
  LEFT JOIN tenants t ON t.id = a.tenant_id
 WHERE a.revoked_at IS NULL
 ORDER BY a.operator_id, a.tenant_id NULLS FIRST;

-- ListAssignmentsForOperator is the guard's lookup, run on EVERY admin call an
-- operator makes.
--
-- It joins platform_users and requires the account to be active, so disabling
-- somebody takes their live tokens down with them: without the join, a
-- disabled operator kept working until their token happened to expire, and
-- "disabled" that does not disable is worse than no button at all.
-- name: ListAssignmentsForOperator :many
SELECT a.id, a.operator_id, a.tenant_id, a.role, a.granted_by, a.reason,
       a.valid_until, a.revoked_at, a.created_at
  FROM platform_assignments a
  JOIN platform_users u ON u.id = a.operator_id
 WHERE a.operator_id = sqlc.arg(operator_id)
   AND a.revoked_at IS NULL
   AND u.status = 'active'
 ORDER BY a.tenant_id NULLS FIRST;

-- name: RevokeAssignment :execrows
UPDATE platform_assignments
   SET revoked_at = now(), updated_at = now()
 WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: HasAnyPlatformOwner :one
SELECT EXISTS (
    SELECT 1 FROM platform_assignments
     WHERE tenant_id IS NULL AND role = 'owner' AND revoked_at IS NULL
) AS present;

-- ---------------------------------------------------------------------------
-- Platform users. A separate population from identities, with no join between
-- them: a tenant's user is not an operator and cannot be made into one.
-- ---------------------------------------------------------------------------

-- name: CreatePlatformUser :one
INSERT INTO platform_users (username, email, password_hash)
VALUES (sqlc.arg(username), sqlc.narg(email), sqlc.arg(password_hash))
RETURNING id;

-- name: GetPlatformUserByUsername :one
SELECT id, username, email, password_hash, status, token_epoch, last_login_at,
       disabled_at, created_at, totp_secret_enc, totp_enrolled_at, totp_last_step
  FROM platform_users WHERE lower(username) = lower(sqlc.arg(username));

-- name: GetPlatformUser :one
SELECT id, username, email, password_hash, status, token_epoch, last_login_at,
       disabled_at, created_at, totp_secret_enc, totp_enrolled_at, totp_last_step
  FROM platform_users WHERE id = sqlc.arg(id);

-- ListPlatformUsers is keyset-paginated on (username, id): OFFSET over a
-- growing table re-scans everything it skips and can show a row twice when
-- one is inserted mid-page.
-- name: ListPlatformUsers :many
SELECT id, username, email, status, token_epoch, last_login_at, disabled_at,
       created_at, totp_enrolled_at
  FROM platform_users
 WHERE (sqlc.arg(query)::text = '' OR username ILIKE '%' || sqlc.arg(query)::text || '%')
   AND (sqlc.arg(after)::text = '' OR username > sqlc.arg(after)::text)
 ORDER BY username
 LIMIT sqlc.arg(page_size);

-- name: CountPlatformUsers :one
SELECT count(*) FROM platform_users;

-- name: SetPlatformUserStatus :execrows
UPDATE platform_users
   SET status = sqlc.arg(status), updated_at = now(),
       disabled_at = CASE WHEN sqlc.arg(status)::text = 'disabled' THEN now() ELSE NULL END,
       token_epoch = token_epoch + CASE WHEN sqlc.arg(status)::text = 'disabled' THEN 1 ELSE 0 END
 WHERE id = sqlc.arg(id);

-- name: TouchPlatformUserLogin :exec
UPDATE platform_users SET last_login_at = now() WHERE id = sqlc.arg(id);

-- StageTotpSecret stores a secret that has NOT been confirmed yet. Enrolment
-- is not complete until a code verifies, so this deliberately leaves
-- totp_enrolled_at alone: holding a secret must never start demanding a
-- factor the operator cannot yet produce.
-- name: StageTotpSecret :execrows
UPDATE platform_users
   SET totp_secret_enc = sqlc.arg(secret), updated_at = now()
 WHERE id = sqlc.arg(id);

-- name: ConfirmTotpEnrolment :execrows
UPDATE platform_users
   SET totp_enrolled_at = now(), totp_last_step = sqlc.arg(step), updated_at = now()
 WHERE id = sqlc.arg(id) AND totp_secret_enc IS NOT NULL;

-- AdvanceTotpStep is the single-use guard: it only succeeds when the step is
-- strictly newer than the last one accepted, so replaying a code inside its
-- own validity window updates nothing and the caller refuses the login.
-- name: AdvanceTotpStep :execrows
UPDATE platform_users
   SET totp_last_step = sqlc.arg(step), updated_at = now()
 WHERE id = sqlc.arg(id) AND totp_last_step < sqlc.arg(step);

-- name: ClearTotp :execrows
UPDATE platform_users
   SET totp_secret_enc = NULL, totp_enrolled_at = NULL, totp_last_step = 0,
       updated_at = now()
 WHERE id = sqlc.arg(id);
