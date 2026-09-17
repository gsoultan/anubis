package tenancyrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// AuthPageRow is one page with its binding resolved for display.
//
// ApplicationSlug and RealmCode come from LEFT joins: a page may be bound to
// an application, to a realm, or to neither, and the check constraint forbids
// both. An inner join would drop the unbound pages, which include every
// tenant default.
type AuthPageRow struct {
	ID              string
	TenantID        string
	Kind            string
	Slug            string
	Name            string
	Status          string
	IsDefault       bool
	ApplicationID   runtime.Null[string]
	ApplicationSlug runtime.Null[string]
	RealmID         runtime.Null[string]
	RealmCode       runtime.Null[string]
	Config          runtime.JSON
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

const pageCols = `
       p.id::text AS id, p.tenant_id::text AS tenant_id, p.kind, p.slug, p.name,
       p.status, p.is_default,
       p.application_id::text AS application_id, a.slug AS application_slug,
       p.realm_id::text AS realm_id, r.code AS realm_code, p.config`

const pageJoins = `
FROM auth_pages p
LEFT JOIN applications a ON a.id = p.application_id
LEFT JOIN realms r ON r.id = p.realm_id`

var ListAuthPages = storm.SQL[AuthPageRow](`
SELECT` + pageCols + `, p.created_at, p.updated_at` + pageJoins + `
WHERE p.tenant_id = $1 AND ($2::text IS NULL OR p.kind = $2)
ORDER BY p.kind, p.is_default DESC, p.slug`)

var GetAuthPage = storm.SQL[AuthPageRow](`
SELECT` + pageCols + `, p.created_at, p.updated_at` + pageJoins + `
WHERE p.id = $1 AND p.tenant_id = $2`)

// ResolvedPageRow is a page on the render path, without the audit timestamps
// nothing on that path displays.
type ResolvedPageRow struct {
	ID              string
	TenantID        string
	Kind            string
	Slug            string
	Name            string
	Status          string
	IsDefault       bool
	ApplicationID   runtime.Null[string]
	ApplicationSlug runtime.Null[string]
	RealmID         runtime.Null[string]
	RealmCode       runtime.Null[string]
	Config          runtime.JSON
}

// GetAuthPageBySlug is the public render path: tenant + kind + slug, active
// only. A disabled page must 404 rather than render, so a retired design
// cannot be resurrected by anyone who kept the link.
var GetAuthPageBySlug = storm.SQL[ResolvedPageRow](`
SELECT` + pageCols + pageJoins + `
WHERE p.tenant_id = $1 AND p.kind = $2 AND p.slug = $3 AND p.status = 'active'`)

// GetDefaultAuthPage is the fallback when nothing more specific binds.
var GetDefaultAuthPage = storm.SQL[ResolvedPageRow](`
SELECT` + pageCols + pageJoins + `
WHERE p.tenant_id = $1 AND p.kind = $2 AND p.is_default AND p.status = 'active'`)

// DeleteAuthPage refuses the default: deleting it would leave /v1/authorize
// with no page to render, so the guard is in the statement rather than in a
// read-then-delete that another writer could interleave with.
//
// SQL because storm generates a delete by PRIMARY KEY only — there is no
// predicate form, and this one needs three conditions.
var DeleteAuthPage = storm.SQLExec(`
DELETE FROM auth_pages
WHERE id = $1 AND tenant_id = $2 AND NOT is_default`)

var UpdateAuthPage = storm.SQLExec(`
UPDATE auth_pages
SET name = $3, status = $4, application_id = $5, realm_id = $6,
    config = $7::jsonb, updated_at = now()
WHERE id = $1 AND tenant_id = $2`)

// ClearDefaultAuthPage demotes whichever page currently holds the slot.
//
// It runs before SetDefaultAuthPage inside one transaction: the partial unique
// index allows exactly one default per kind, so promoting without demoting
// first is a constraint violation rather than a swap.
var ClearDefaultAuthPage = storm.SQLExec(`
UPDATE auth_pages SET is_default = false, updated_at = now()
WHERE tenant_id = $1 AND kind = $2 AND is_default`)

// SetDefaultAuthPage promotes a page, activating it in the same statement: a
// default that is disabled is a door the tenant cannot open.
var SetDefaultAuthPage = storm.SQLExec(`
UPDATE auth_pages SET is_default = true, status = 'active', updated_at = now()
WHERE id = $1 AND tenant_id = $2`)
