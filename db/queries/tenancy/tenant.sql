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


-- name: UpdateTenant :execrows
UPDATE tenants SET name = sqlc.arg(name), updated_at = now()
 WHERE id = sqlc.arg(id);

-- SetTenantStatus is how a tenant is suspended or retired. There is no DELETE:
-- every grant, identity, scope node and audit record in the installation hangs
-- off this row, and dropping it would take the evidence with it. 'archived' is
-- what "delete" means here.
-- name: SetTenantStatus :execrows
UPDATE tenants SET status = sqlc.arg(status), updated_at = now()
 WHERE id = sqlc.arg(id);

-- name: CountTenantIdentities :one
SELECT count(*) FROM identities i WHERE i.tenant_id = sqlc.arg(id);

-- TenantStats is the "what is in here" summary the tenants page shows beside
-- each row. Counted live rather than cached: a stale number next to a tenant
-- somebody is about to retire is worse than no number.
-- name: GetTenantStats :one
SELECT
  (SELECT count(*) FROM identities  i WHERE i.tenant_id = sqlc.arg(id)) AS identities,
  (SELECT count(*) FROM grants      g WHERE g.tenant_id = sqlc.arg(id) AND g.revoked_at IS NULL) AS grants,
  (SELECT count(*) FROM scope_nodes n WHERE n.tenant_id = sqlc.arg(id) AND n.status = 'active') AS scope_nodes,
  (SELECT count(*) FROM memberships m WHERE m.tenant_id = sqlc.arg(id)) AS memberships;
