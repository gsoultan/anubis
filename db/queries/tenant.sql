-- name: GetTenantBySlug :one
SELECT id, slug, name, status, created_at FROM tenants WHERE slug = sqlc.arg(slug);

-- name: GetTenant :one
SELECT id, slug, name, status, created_at FROM tenants WHERE id = sqlc.arg(id);

-- name: ListTenants :many
SELECT id, slug, name, status, created_at FROM tenants ORDER BY slug;

-- name: CreateTenant :one
INSERT INTO tenants (slug, name)
VALUES (sqlc.arg(slug), sqlc.arg(name))
RETURNING id, slug, name, status, created_at;

-- name: GetCatalogVersion :one
SELECT version, changed_at FROM catalog_version WHERE tenant_id = sqlc.arg(tenant_id);

-- name: BumpCatalogVersion :exec
SELECT bump_catalog_version(sqlc.arg(tenant_id));
