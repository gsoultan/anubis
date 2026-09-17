package tenancyrquery

import (
	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// ApplicationRow is one application as the API presents it.
//
// The TTLs arrive twice on purpose: as text, which is what an operator edits
// and what round-trips exactly, and as seconds, which is what a token issuer
// arithmetics on. Deriving one from the other in Go would mean parsing
// PostgreSQL's interval grammar, which is a larger thing than it sounds.
type ApplicationRow struct {
	ID                     string
	TenantID               string
	Slug                   string
	Name                   string
	Kind                   string
	Status                 string
	RedirectUris           []string
	PostLogoutRedirectUris []string
	BackchannelLogoutURI   runtime.Null[string]
	TokenFormat            string
	ClientSecretHash       runtime.Null[string]
	ManifestVersion        int32
	AccessTokenTtl         string
	RefreshTokenTtl        string
}

const appCols = `
       id::text AS id, tenant_id::text AS tenant_id, slug, name, kind, status,
       redirect_uris, post_logout_redirect_uris, backchannel_logout_uri,
       token_format, client_secret_hash, manifest_version,
       access_token_ttl::text AS access_token_ttl,
       refresh_token_ttl::text AS refresh_token_ttl`

// ApplicationWithSecsRow is an application plus its TTLs in seconds.
//
// The fields are repeated rather than embedded: storm's raw scanner maps
// result columns to fields by name and does not descend into an embedded
// struct, so an embedded ApplicationRow is "fed by no result column".
type ApplicationWithSecsRow struct {
	ID                     string
	TenantID               string
	Slug                   string
	Name                   string
	Kind                   string
	Status                 string
	RedirectUris           []string
	PostLogoutRedirectUris []string
	BackchannelLogoutURI   runtime.Null[string]
	TokenFormat            string
	ClientSecretHash       runtime.Null[string]
	ManifestVersion        int32
	AccessTokenTtl         string
	RefreshTokenTtl        string
	AccessTokenTtlSecs     int64
	RefreshTokenTtlSecs    int64
}

// GetApplicationBySlug is the sign-in path's lookup, which also needs the TTLs
// as seconds to stamp a token's expiry.

var GetApplicationBySlug = storm.SQL[ApplicationWithSecsRow](`
SELECT` + appCols + `,
       extract(epoch FROM access_token_ttl)::bigint  AS access_token_ttl_secs,
       extract(epoch FROM refresh_token_ttl)::bigint AS refresh_token_ttl_secs
FROM applications
WHERE tenant_id = $1 AND slug = $2`)

var GetApplication = storm.SQL[ApplicationRow](`
SELECT` + appCols + `
FROM applications
WHERE id = $1 AND tenant_id = $2`)

// ListApplications is keyset-paginated on slug, with an optional search.
//
// The search stays SQL because the wildcards are not the caller's: building
// '%' || $n || '%' in Go would put a pattern somebody typed into a LIKE
// without anybody reading it as one.
var ListApplications = storm.SQL[ApplicationRow](`
SELECT` + appCols + `
FROM applications
WHERE tenant_id = $1
  AND ($2::text = '' OR slug ILIKE '%' || $2::text || '%' OR name ILIKE '%' || $2::text || '%')
  AND ($3::text = '' OR slug > $3::text)
ORDER BY slug
LIMIT $4`)

// AllApplications is the unpaged read for internal checks: validating a
// post-logout redirect walks every registered URI, and paging that would
// silently reject valid redirects past the first page.
var AllApplications = storm.SQL[ApplicationRow](`
SELECT` + appCols + `
FROM applications
WHERE tenant_id = $1
ORDER BY slug`)

// CreatedApplicationRow is a new application's id and starting version.
type CreatedApplicationRow struct {
	ID              string
	ManifestVersion int32
}

// CreateApplication writes an application with its TTLs as intervals.
//
// nullif on the two optional strings: ” and "absent" are different facts for
// a client secret — one means a public client, the other an empty password.
var CreateApplication = storm.SQL[CreatedApplicationRow](`
INSERT INTO applications (tenant_id, slug, name, kind, redirect_uris,
                          post_logout_redirect_uris, backchannel_logout_uri,
                          token_format, client_secret_hash,
                          access_token_ttl, refresh_token_ttl)
VALUES ($1, $2, $3, $4, $5::text[], $6::text[], nullif($7, ''), $8,
        nullif($9, ''), $10::text::interval, $11::text::interval)
RETURNING id::text AS id, manifest_version`)

var UpdateApplication = storm.SQLExec(`
UPDATE applications
SET name = $3, status = $4, redirect_uris = $5::text[],
    post_logout_redirect_uris = $6::text[],
    backchannel_logout_uri = nullif($7, ''), token_format = $8,
    access_token_ttl = $9::text::interval,
    refresh_token_ttl = $10::text::interval,
    updated_at = now()
WHERE id = $1 AND tenant_id = $2`)

var SetClientSecretHash = storm.SQLExec(`
UPDATE applications SET client_secret_hash = $3, updated_at = now()
WHERE id = $1 AND tenant_id = $2`)

// ManifestVersionRow is the version after a bump.
type ManifestVersionRow struct {
	ManifestVersion int32
}

// BumpManifestVersion publishes a new configuration generation.
//
// manifest_version + 1 is computed by the database: in Go it is a
// read-modify-write, and two concurrent edits would both publish N+1 while a
// client cache keyed on the version kept one of them indefinitely.
var BumpManifestVersion = storm.SQL[ManifestVersionRow](`
UPDATE applications SET manifest_version = manifest_version + 1, updated_at = now()
WHERE id = $1
RETURNING manifest_version`)

// DeleteRoutePoliciesByApp clears an application's whole rule set before a
// replace. SQL because storm's generated delete takes a primary key, and this
// deletes every row for one application.
var DeleteRoutePoliciesByApp = storm.SQLExec(`
DELETE FROM route_policies WHERE application_id = $1`)

// RoutePolicyRow is one gate rule with its permission key resolved.
type RoutePolicyRow struct {
	ID            string
	ApplicationID string
	Priority      int32
	Effect        string
	PathPattern   string
	HostPattern   runtime.Null[string]
	Methods       []string
	ScopeBindings runtime.JSON
	PermissionKey runtime.Null[string]
}

// ListRoutePoliciesByApp reads an application's rules in evaluation order.
//
// LEFT JOIN: only a require_permission rule names a permission, and an inner
// join would silently drop every public and deny rule — which is most of them.
var ListRoutePoliciesByApp = storm.SQL[RoutePolicyRow](`
SELECT rp.id::text AS id, rp.application_id::text AS application_id,
       rp.priority, rp.effect, rp.path_pattern, rp.host_pattern,
       rp.methods, rp.scope_bindings, p.key AS permission_key
FROM route_policies rp
LEFT JOIN permissions p ON p.id = rp.permission_id
WHERE rp.application_id = $1
ORDER BY rp.priority`)
