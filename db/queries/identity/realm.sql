-- name: GetRealmByCode :one
SELECT id, tenant_id, code, kind, display_name, min_assurance, self_registration,
       email_verification_required, pii_encryption,
       allowed_factors, required_factors, password_policy,
       factor_enrolment_deadline,
       session_ttl::text  AS session_ttl,
       access_token_ttl::text AS access_token_ttl,
       refresh_token_ttl::text AS refresh_token_ttl,
       COALESCE(default_retention::text, '')::text AS default_retention,
       extract(epoch FROM session_ttl)::bigint       AS session_ttl_secs,
       extract(epoch FROM access_token_ttl)::bigint  AS access_token_ttl_secs,
       extract(epoch FROM refresh_token_ttl)::bigint AS refresh_token_ttl_secs
FROM realms
WHERE tenant_id = sqlc.arg(tenant_id) AND code = sqlc.arg(code);

-- name: GetRealm :one
SELECT id, tenant_id, code, kind, display_name, min_assurance, self_registration,
       email_verification_required, pii_encryption,
       allowed_factors, required_factors, password_policy,
       factor_enrolment_deadline,
       session_ttl::text  AS session_ttl,
       access_token_ttl::text AS access_token_ttl,
       refresh_token_ttl::text AS refresh_token_ttl,
       COALESCE(default_retention::text, '')::text AS default_retention,
       extract(epoch FROM session_ttl)::bigint       AS session_ttl_secs,
       extract(epoch FROM access_token_ttl)::bigint  AS access_token_ttl_secs,
       extract(epoch FROM refresh_token_ttl)::bigint AS refresh_token_ttl_secs
FROM realms
WHERE id = sqlc.arg(id);

-- name: ListRealms :many
SELECT id, tenant_id, code, kind, display_name, min_assurance, self_registration,
       email_verification_required, pii_encryption,
       allowed_factors, required_factors, password_policy,
       factor_enrolment_deadline,
       session_ttl::text  AS session_ttl,
       access_token_ttl::text AS access_token_ttl,
       refresh_token_ttl::text AS refresh_token_ttl,
       COALESCE(default_retention::text, '')::text AS default_retention
FROM realms
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY code;

-- name: CreateRealm :one
INSERT INTO realms (tenant_id, code, kind, display_name, min_assurance,
                    self_registration, email_verification_required, pii_encryption,
                    allowed_factors, required_factors, password_policy,
                    session_ttl, access_token_ttl, refresh_token_ttl, default_retention,
                    factor_enrolment_deadline)
VALUES (sqlc.arg(tenant_id), sqlc.arg(code), sqlc.arg(kind), sqlc.arg(display_name),
        sqlc.arg(min_assurance), sqlc.arg(self_registration),
        sqlc.arg(email_verification_required), sqlc.arg(pii_encryption),
        sqlc.arg(allowed_factors)::text[], sqlc.arg(required_factors)::text[],
        sqlc.arg(password_policy)::jsonb,
        sqlc.arg(session_ttl)::text::interval, sqlc.arg(access_token_ttl)::text::interval,
        sqlc.arg(refresh_token_ttl)::text::interval,
        nullif(sqlc.arg(default_retention), '')::interval,
        sqlc.narg(factor_enrolment_deadline)::timestamptz)
RETURNING id;

-- name: UpdateRealm :one
UPDATE realms SET
    display_name = sqlc.arg(display_name),
    min_assurance = sqlc.arg(min_assurance),
    self_registration = sqlc.arg(self_registration),
    email_verification_required = sqlc.arg(email_verification_required),
    pii_encryption = sqlc.arg(pii_encryption),
    allowed_factors = sqlc.arg(allowed_factors)::text[],
    required_factors = sqlc.arg(required_factors)::text[],
    password_policy = sqlc.arg(password_policy)::jsonb,
    session_ttl = sqlc.arg(session_ttl)::text::interval,
    access_token_ttl = sqlc.arg(access_token_ttl)::text::interval,
    refresh_token_ttl = sqlc.arg(refresh_token_ttl)::text::interval,
    default_retention = nullif(sqlc.arg(default_retention), '')::interval,
    -- NULL takes the policy out of force again; enrolments already made
    -- survive, so a second rollout starts ahead of the first.
    factor_enrolment_deadline = sqlc.narg(factor_enrolment_deadline)::timestamptz,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id;

-- name: ListRealmCategories :many
SELECT id, realm_id, code, display_name, sort_order
FROM realm_categories
WHERE realm_id = sqlc.arg(realm_id)
ORDER BY sort_order, code;

-- name: GetRealmCategoryByCode :one
SELECT id, realm_id, code, display_name, sort_order
FROM realm_categories
WHERE realm_id = sqlc.arg(realm_id) AND code = sqlc.arg(code);

-- name: CreateRealmCategory :one
INSERT INTO realm_categories (tenant_id, realm_id, code, display_name, sort_order)
VALUES (sqlc.arg(tenant_id), sqlc.arg(realm_id), sqlc.arg(code),
        sqlc.arg(display_name), sqlc.arg(sort_order))
RETURNING id, realm_id, code, display_name, sort_order;
