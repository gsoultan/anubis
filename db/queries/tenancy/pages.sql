-- Sign-in and sign-out pages (migrations/0024). A tenant may have many of
-- each; exactly one per kind is the default, enforced by a partial unique
-- index rather than by application code.

-- name: ListAuthPages :many
SELECT p.id, p.tenant_id, p.kind, p.slug, p.name, p.status, p.is_default,
       p.application_id, a.slug AS application_slug, p.realm_id, r.code AS realm_code, p.config,
       p.created_at, p.updated_at
FROM auth_pages p
LEFT JOIN applications a ON a.id = p.application_id
LEFT JOIN realms r ON r.id = p.realm_id
WHERE p.tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.narg(kind)::text IS NULL OR p.kind = sqlc.narg(kind))
ORDER BY p.kind, p.is_default DESC, p.slug;

-- name: GetAuthPage :one
SELECT p.id, p.tenant_id, p.kind, p.slug, p.name, p.status, p.is_default,
       p.application_id, a.slug AS application_slug, p.realm_id, r.code AS realm_code, p.config,
       p.created_at, p.updated_at
FROM auth_pages p
LEFT JOIN applications a ON a.id = p.application_id
LEFT JOIN realms r ON r.id = p.realm_id
WHERE p.id = sqlc.arg(id) AND p.tenant_id = sqlc.arg(tenant_id);

-- name: GetAuthPageBySlug :one
-- The public render path: tenant + kind + slug, active only. A disabled page
-- must 404 rather than render, so a retired design cannot be resurrected by
-- anyone who kept the link.
SELECT p.id, p.tenant_id, p.kind, p.slug, p.name, p.status, p.is_default,
       p.application_id, a.slug AS application_slug, p.realm_id, r.code AS realm_code, p.config
FROM auth_pages p
LEFT JOIN applications a ON a.id = p.application_id
LEFT JOIN realms r ON r.id = p.realm_id
WHERE p.tenant_id = sqlc.arg(tenant_id) AND p.kind = sqlc.arg(kind)
  AND p.slug = sqlc.arg(slug) AND p.status = 'active';

-- name: GetDefaultAuthPage :one
SELECT p.id, p.tenant_id, p.kind, p.slug, p.name, p.status, p.is_default,
       p.application_id, a.slug AS application_slug, p.realm_id, r.code AS realm_code, p.config
FROM auth_pages p
LEFT JOIN applications a ON a.id = p.application_id
LEFT JOIN realms r ON r.id = p.realm_id
WHERE p.tenant_id = sqlc.arg(tenant_id) AND p.kind = sqlc.arg(kind)
  AND p.is_default AND p.status = 'active';

-- name: GetAuthPageForApplication :one
-- An application-initiated flow prefers that application's own page.
SELECT p.id, p.tenant_id, p.kind, p.slug, p.name, p.status, p.is_default,
       p.application_id, p.realm_id, p.config
FROM auth_pages p
WHERE p.tenant_id = sqlc.arg(tenant_id) AND p.kind = sqlc.arg(kind)
  AND p.application_id = sqlc.arg(application_id) AND p.status = 'active';

-- name: GetAuthPageForRealm :one
-- The population's own door. Tried AFTER the application binding and BEFORE
-- the tenant default, so an application that configured its own page keeps
-- it; this fills the gap that used to fall straight through to the default.
SELECT p.id, p.tenant_id, p.kind, p.slug, p.name, p.status, p.is_default,
       p.application_id, p.realm_id, p.config
FROM auth_pages p
WHERE p.tenant_id = sqlc.arg(tenant_id) AND p.kind = sqlc.arg(kind)
  AND p.realm_id = sqlc.arg(realm_id) AND p.status = 'active';

-- name: CreateAuthPage :one
INSERT INTO auth_pages (tenant_id, kind, slug, name, status, application_id,
                        realm_id, config)
VALUES (sqlc.arg(tenant_id), sqlc.arg(kind), sqlc.arg(slug), sqlc.arg(name),
        sqlc.arg(status), sqlc.narg(application_id), sqlc.narg(realm_id),
        sqlc.arg(config)::jsonb)
RETURNING id;

-- name: UpdateAuthPage :execrows
UPDATE auth_pages
SET name = sqlc.arg(name),
    status = sqlc.arg(status),
    application_id = sqlc.narg(application_id),
    realm_id = sqlc.narg(realm_id),
    config = sqlc.arg(config)::jsonb,
    updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: DeleteAuthPage :execrows
-- The default page is not deletable: deleting it would leave /v1/authorize
-- with no page to render. Promote another page first.
DELETE FROM auth_pages
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id) AND NOT is_default;

-- name: ClearDefaultAuthPage :exec
UPDATE auth_pages SET is_default = false, updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id) AND kind = sqlc.arg(kind) AND is_default;

-- name: SetDefaultAuthPage :execrows
UPDATE auth_pages SET is_default = true, status = 'active', updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);
