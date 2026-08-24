-- name: GetApplicationBySlug :one
SELECT id, tenant_id, slug, name, kind, status, redirect_uris,
       post_logout_redirect_uris,
       backchannel_logout_uri, token_format, client_secret_hash,
       manifest_version,
       access_token_ttl::text AS access_token_ttl,
       refresh_token_ttl::text AS refresh_token_ttl,
       extract(epoch FROM access_token_ttl)::bigint  AS access_token_ttl_secs,
       extract(epoch FROM refresh_token_ttl)::bigint AS refresh_token_ttl_secs
FROM applications
WHERE tenant_id = sqlc.arg(tenant_id) AND slug = sqlc.arg(slug);

-- name: GetApplication :one
SELECT id, tenant_id, slug, name, kind, status, redirect_uris,
       post_logout_redirect_uris,
       backchannel_logout_uri, token_format, client_secret_hash,
       manifest_version,
       access_token_ttl::text AS access_token_ttl,
       refresh_token_ttl::text AS refresh_token_ttl
FROM applications
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- ListApplications is the TENANT's relying parties: the things its people
-- sign in to. Since 0029 nothing else lives in this table — Anubis itself
-- registers no applications anywhere.
-- name: ListApplications :many
SELECT id, tenant_id, slug, name, kind, status, redirect_uris,
       post_logout_redirect_uris,
       backchannel_logout_uri, token_format, manifest_version,
       access_token_ttl::text AS access_token_ttl,
       refresh_token_ttl::text AS refresh_token_ttl
FROM applications
WHERE tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(query)::text = ''
       OR slug ILIKE '%' || sqlc.arg(query)::text || '%'
       OR name ILIKE '%' || sqlc.arg(query)::text || '%')
  AND (sqlc.arg(after)::text = '' OR slug > sqlc.arg(after)::text)
ORDER BY slug
LIMIT sqlc.arg(page_size);

-- AllApplications is the unpaged read for internal checks: validating a
-- post-logout redirect walks every registered URI, and paging that would
-- silently reject valid redirects past the first page.
-- name: AllApplications :many
SELECT id, tenant_id, slug, name, kind, status, redirect_uris,
       post_logout_redirect_uris,
       backchannel_logout_uri, token_format, manifest_version,
       access_token_ttl::text AS access_token_ttl,
       refresh_token_ttl::text AS refresh_token_ttl
FROM applications
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY slug;

-- name: CountApplications :one
SELECT count(*) FROM applications
 WHERE tenant_id = sqlc.arg(tenant_id);

-- name: CreateApplication :one
INSERT INTO applications (tenant_id, slug, name, kind, redirect_uris,
                          post_logout_redirect_uris,
                          backchannel_logout_uri, token_format,
                          client_secret_hash, access_token_ttl, refresh_token_ttl)
VALUES (sqlc.arg(tenant_id), sqlc.arg(slug), sqlc.arg(name), sqlc.arg(kind),
        sqlc.arg(redirect_uris)::text[],
        sqlc.arg(post_logout_redirect_uris)::text[],
        nullif(sqlc.arg(backchannel_logout_uri), ''),
        sqlc.arg(token_format),
        nullif(sqlc.arg(client_secret_hash), ''),
        sqlc.arg(access_token_ttl)::text::interval,
        sqlc.arg(refresh_token_ttl)::text::interval)
RETURNING id, manifest_version;

-- name: UpdateApplication :execrows
UPDATE applications
SET name = sqlc.arg(name),
    status = sqlc.arg(status),
    redirect_uris = sqlc.arg(redirect_uris)::text[],
    post_logout_redirect_uris = sqlc.arg(post_logout_redirect_uris)::text[],
    backchannel_logout_uri = nullif(sqlc.arg(backchannel_logout_uri), ''),
    token_format = sqlc.arg(token_format),
    access_token_ttl = sqlc.arg(access_token_ttl)::text::interval,
    refresh_token_ttl = sqlc.arg(refresh_token_ttl)::text::interval,
    updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: SetClientSecretHash :execrows
UPDATE applications SET client_secret_hash = sqlc.arg(client_secret_hash),
       updated_at = now()
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: BumpManifestVersion :one
UPDATE applications SET manifest_version = manifest_version + 1, updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING manifest_version;

-- name: ListRoutePoliciesByApp :many
SELECT rp.id, rp.application_id, rp.priority, rp.effect, rp.path_pattern,
       rp.host_pattern, rp.methods, rp.scope_bindings,
       p.key AS permission_key
FROM route_policies rp
LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE rp.application_id = sqlc.arg(application_id)
ORDER BY rp.priority;

-- name: DeleteRoutePoliciesByApp :exec
DELETE FROM route_policies WHERE application_id = sqlc.arg(application_id);

-- name: InsertRoutePolicy :exec
INSERT INTO route_policies (application_id, tenant_id, permission_id, priority,
                            effect, path_pattern, host_pattern, methods,
                            scope_bindings)
VALUES (sqlc.arg(application_id), sqlc.arg(tenant_id), sqlc.narg(permission_id),
        sqlc.arg(priority), sqlc.arg(effect), sqlc.arg(path_pattern),
        nullif(sqlc.arg(host_pattern), ''), sqlc.arg(methods)::text[],
        sqlc.arg(scope_bindings)::jsonb);

-- name: ListBackchannelApps :many
SELECT slug, backchannel_logout_uri
FROM applications
WHERE tenant_id = sqlc.arg(tenant_id)
  AND backchannel_logout_uri IS NOT NULL
  AND status = 'active';
