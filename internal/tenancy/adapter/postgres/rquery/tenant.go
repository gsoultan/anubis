package tenancyrquery

import "github.com/gsoultan/storm"

var UpdateTenant = storm.SQLExec(`
UPDATE tenants SET name = $2, updated_at = now() WHERE id = $1`)

// SetTenantStatus suspends or retires a tenant. There is no DELETE: every
// grant, identity, scope node and audit record in the installation hangs off
// this row, and dropping it would take the evidence with it. 'archived' is
// what "delete" means here.
var SetTenantStatus = storm.SQLExec(`
UPDATE tenants SET status = $2, updated_at = now() WHERE id = $1`)

// DoneRow carries the result of a void-returning call. `IS NULL` turns void
// into a scannable boolean, because storm.SQLExec rightly refuses a statement
// whose descriptor has columns and a SELECT of a function call always has one.
type DoneRow struct {
	Done bool
}

// BumpCatalogVersion invalidates every gate snapshot for a tenant.
var BumpCatalogVersion = storm.SQL[DoneRow](`
SELECT (bump_catalog_version($1) IS NULL) AS done`)

// CountRow is a single count.
type CountRow struct {
	N int64
}

// CountTenantIdentities backs the "this holds N people" warning shown before a
// tenant is retired. identities belongs to the identity context, so this
// counts across a boundary and stays SQL.
var CountTenantIdentities = storm.SQL[CountRow](`
SELECT count(*) AS n FROM identities i WHERE i.tenant_id = $1`)

// TenantStatsRow is the "what is in here" summary the tenants page shows.
type TenantStatsRow struct {
	Identities  int64
	Grants      int64
	ScopeNodes  int64
	Memberships int64
}

// GetTenantStats counts live rather than caching: a stale number next to a
// tenant somebody is about to retire is worse than no number.
//
// Four scalar subqueries in one statement, so the four counts describe one
// moment. Four separate queries would not.
var GetTenantStats = storm.SQL[TenantStatsRow](`
SELECT
  (SELECT count(*) FROM identities  i WHERE i.tenant_id = $1) AS identities,
  (SELECT count(*) FROM grants      g WHERE g.tenant_id = $1 AND g.revoked_at IS NULL) AS grants,
  (SELECT count(*) FROM scope_nodes n WHERE n.tenant_id = $1 AND n.status = 'active') AS scope_nodes,
  (SELECT count(*) FROM memberships m WHERE m.tenant_id = $1) AS memberships`)
