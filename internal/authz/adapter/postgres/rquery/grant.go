package authzrquery

import (
	"time"

	"github.com/gsoultan/raorm"
	"github.com/gsoultan/raorm/runtime"
)

// GrantRow is a grant with its role name, the ListGrantsByIdentity shape.
type GrantRow struct {
	ID              string
	IdentityID      string
	RoleID          string
	RoleName        string
	SelfScoped      bool
	ValidFrom       time.Time
	ValidUntil      runtime.Null[time.Time]
	RevokedAt       runtime.Null[time.Time]
	GrantedBy       string
	Reason          runtime.Null[string]
	ViaMembershipID runtime.Null[string]
}

// ListGrantsByIdentity lists an identity's grants, optionally with revoked
// history. $3 include_revoked.
var ListGrantsByIdentity = raorm.SQL[GrantRow](`
SELECT g.id::text AS id, g.identity_id::text AS identity_id,
       g.role_id::text AS role_id, r.name AS role_name, g.self_scoped,
       g.valid_from, g.valid_until, g.revoked_at,
       g.granted_by::text AS granted_by, g.reason,
       g.via_membership_id::text AS via_membership_id
FROM grants g
JOIN roles r ON r.id = g.role_id
WHERE g.identity_id = $1 AND g.tenant_id = $2
  AND ($3::boolean OR g.revoked_at IS NULL)
ORDER BY g.created_at DESC`)

// GrantScopeRow is one grant's scope pin with its node name.
type GrantScopeRow struct {
	GrantID     string
	AxisCode    string
	ScopeNodeID string
	Inherit     bool
	NodeName    string
}

// ListGrantScopes fans out over a page of grants in ONE query.
var ListGrantScopes = raorm.SQL[GrantScopeRow](`
SELECT gs.grant_id::text AS grant_id, gs.axis_code,
       gs.scope_node_id::text AS scope_node_id, gs.inherit,
       sn.name AS node_name
FROM grant_scopes gs
JOIN scope_nodes sn ON sn.id = gs.scope_node_id
WHERE gs.grant_id = ANY($1::uuid[])
ORDER BY gs.grant_id, gs.axis_code, sn.name`)

// CreatedGrantRow is a fresh grant's identity.
type CreatedGrantRow struct {
	ID        string
	ValidFrom time.Time
}

// CreateGrant inserts the grant row; scopes follow in the same transaction.
// $5 reason (” becomes NULL), $7 valid_until (nil for open-ended).
var CreateGrant = raorm.SQL[CreatedGrantRow](`
INSERT INTO grants (tenant_id, identity_id, role_id, granted_by, reason,
                    self_scoped, valid_until)
VALUES ($1, $2, $3, $4, nullif($5, ''), $6, $7)
RETURNING id::text AS id, valid_from`)

// InsertGrantScope pins one axis of a grant.
var InsertGrantScope = raorm.SQLExec(`
INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id, inherit)
VALUES ($1, $2, $3, $4, $5)`)

// RevokedGrantRow identifies the grant a revocation touched.
type RevokedGrantRow struct {
	ID         string
	IdentityID string
	RoleID     string
}

// RevokeGrant stamps revoked_at once; an already-revoked grant is not found.
// $3 reason (” keeps the original).
var RevokeGrant = raorm.SQL[RevokedGrantRow](`
UPDATE grants
SET revoked_at = now(),
    reason = CASE WHEN $3 = '' THEN reason ELSE $3 END
WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL
RETURNING id::text AS id, identity_id::text AS identity_id,
          role_id::text AS role_id`)

// SearchGrantRow is a grant hit on the Access screen: the grant, its role
// name, and the holder's username.
type SearchGrantRow struct {
	ID              string
	IdentityID      string
	Username        string
	RoleID          string
	RoleName        string
	SelfScoped      bool
	ValidFrom       time.Time
	ValidUntil      runtime.Null[time.Time]
	RevokedAt       runtime.Null[time.Time]
	GrantedBy       string
	Reason          runtime.Null[string]
	ViaMembershipID runtime.Null[string]
	CreatedAt       time.Time
}

// SearchGrants backs the Access screen.
//
// There is deliberately no "list every grant": a tenant here holds 150k of
// them, and a screen that asked for all of them would be answering a question
// nobody can read. Filters narrow first, keyset paging carries the rest.
// Ordered by (created_at, id) so the cursor is stable when two grants share a
// timestamp — which they do, in bulk imports. $1 tenant, $2 include_revoked,
// $3 identity filter, $4 role filter, $5 source ('direct'|'membership'|”),
// $6 text query, $7 cursor grant id, $8 page size.
var SearchGrants = raorm.SQL[SearchGrantRow](`
SELECT g.id::text AS id, g.identity_id::text AS identity_id, i.username,
       g.role_id::text AS role_id, r.name AS role_name,
       g.self_scoped, g.valid_from, g.valid_until, g.revoked_at,
       g.granted_by::text AS granted_by, g.reason,
       g.via_membership_id::text AS via_membership_id, g.created_at
  FROM grants g
  JOIN roles r      ON r.id = g.role_id
  JOIN identities i ON i.id = g.identity_id
 WHERE g.tenant_id = $1
   AND ($2::boolean OR g.revoked_at IS NULL)
   AND ($3::text = '' OR g.identity_id::text = $3::text)
   AND ($4::text = ''     OR g.role_id::text = $4::text)
   AND ($5::text <> 'direct'     OR g.via_membership_id IS NULL)
   AND ($5::text <> 'membership' OR g.via_membership_id IS NOT NULL)
   AND ($6::text = ''
        OR i.username ILIKE '%' || $6::text || '%'
        OR r.name     ILIKE '%' || $6::text || '%')
   AND ($7::text = ''
        OR (g.created_at, g.id) < (
              SELECT g2.created_at, g2.id FROM grants g2
               WHERE g2.id::text = $7::text))
 ORDER BY g.created_at DESC, g.id DESC
 LIMIT $8`)

// CountRow is a bare count.
type CountRow struct {
	Count int64
}

// CountLiveGrants backs the dashboard: access currently in force.
var CountLiveGrants = raorm.SQL[CountRow](`
SELECT count(*) AS count FROM grants
WHERE tenant_id = $1 AND revoked_at IS NULL
  AND (valid_until IS NULL OR valid_until > now())`)
