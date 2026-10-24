package authzrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// AccessSummaryRow is one person's live access, counted.
type AccessSummaryRow struct {
	IdentityID string
	Direct     int32
	Groups     int32
	Roles      []string
	GroupNames []string
	NextEnd    runtime.Null[time.Time]
}

// SummarizeAccess backs the People list's "Can do" column: one query for a
// page of people. Role and group names are capped at six each; the screen
// shows two or three and a count.
var SummarizeAccess = storm.SQL[AccessSummaryRow](`
SELECT g.identity_id::text AS identity_id,
       (count(*) FILTER (WHERE g.via_membership_id IS NULL))::int4 AS direct,
       (count(DISTINCT g.via_membership_id))::int4 AS groups,
       (array_agg(DISTINCT r.name ORDER BY r.name))[1:6]::text[] AS roles,
       coalesce((array_agg(DISTINCT m.name ORDER BY m.name)
                 FILTER (WHERE m.name IS NOT NULL))[1:6], '{}')::text[] AS group_names,
       min(g.valid_until) AS next_end
  FROM grants g
  JOIN roles r ON r.id = g.role_id
  LEFT JOIN memberships m ON m.id = g.via_membership_id
 WHERE g.tenant_id = $1 AND g.identity_id = ANY($2::uuid[])
   AND g.revoked_at IS NULL
   AND (g.valid_until IS NULL OR g.valid_until > now())
 GROUP BY g.identity_id`)

// FullListRow is grants of one role naming every place under one parent.
type FullListRow struct {
	AxisCode   string
	ParentID   string
	ParentName string
	IsRoot     bool
	RoleID     string
	RoleName   string
	Grants     int32
	Places     int32
}

// ListFullLists finds hand-picked lists that name every live place under a
// parent one by one (a parent with at least two). Each reads as "all" today
// and silently misses the next place added there. Direct grants only:
// a group's list is the group's to fix. $2 axis, $3 role (” for any).
var ListFullLists = storm.SQL[FullListRow](`
WITH picks AS (
  SELECT g.id AS grant_id, g.role_id, gs.axis_code, sn.parent_id, gs.scope_node_id
    FROM grants g
    JOIN grant_scopes gs ON gs.grant_id = g.id AND gs.mode = 'include'
    JOIN scope_nodes sn  ON sn.id = gs.scope_node_id AND sn.status = 'active'
   WHERE g.tenant_id = $1 AND g.revoked_at IS NULL
     AND (g.valid_until IS NULL OR g.valid_until > now())
     AND g.via_membership_id IS NULL
     AND sn.parent_id IS NOT NULL
     AND ($2::text = '' OR gs.axis_code = $2::text)
     AND ($3::text = '' OR g.role_id::text = $3::text)
), kids AS (
  SELECT parent_id, count(*) AS n FROM scope_nodes
   WHERE tenant_id = $1 AND status = 'active' AND parent_id IS NOT NULL
   GROUP BY parent_id HAVING count(*) >= 2
), full_lists AS (
  SELECT p.grant_id, p.role_id, p.axis_code, p.parent_id, k.n
    FROM picks p JOIN kids k USING (parent_id)
   GROUP BY p.grant_id, p.role_id, p.axis_code, p.parent_id, k.n
  HAVING count(DISTINCT p.scope_node_id) = k.n
)
SELECT f.axis_code, f.parent_id::text AS parent_id, pn.name AS parent_name,
       pn.is_axis_root AS is_root, f.role_id::text AS role_id, r.name AS role_name,
       count(*)::int4 AS grants, max(f.n)::int4 AS places
  FROM full_lists f
  JOIN scope_nodes pn ON pn.id = f.parent_id
  JOIN roles r ON r.id = f.role_id
 GROUP BY f.axis_code, f.parent_id, pn.name, pn.is_axis_root, f.role_id, r.name
 ORDER BY count(*) DESC, r.name
 LIMIT 100`)

// reachCTE is the place, its siblings, and every live direct grant's picks
// on its kind of place, scored against them. Shared by the reach queries so
// "names every sibling" means the same thing in each.
const reachCTE = `
WITH n AS (
  SELECT sn.id, sn.axis_code, sn.parent_id FROM scope_nodes sn
   WHERE sn.id::text = $2::text AND sn.tenant_id = $1
), sib AS (
  SELECT s.id FROM scope_nodes s, n
   WHERE s.parent_id = n.parent_id AND s.id <> n.id AND s.status = 'active'
), nsib AS (
  SELECT count(*) AS c FROM sib
), per AS (
  SELECT g.id AS grant_id, g.identity_id, g.role_id,
         count(*) FILTER (WHERE gs.scope_node_id IN (SELECT id FROM sib)) AS hit,
         bool_or(EXISTS (SELECT 1 FROM scope_closure c
                          WHERE c.ancestor_id = gs.scope_node_id AND c.descendant_id = n.id
                            AND (gs.inherit OR c.depth = 0))) AS has_it
    FROM grants g
    JOIN grant_scopes gs ON gs.grant_id = g.id AND gs.mode = 'include'
    JOIN n ON gs.axis_code = n.axis_code
   WHERE g.tenant_id = $1 AND g.revoked_at IS NULL
     AND (g.valid_until IS NULL OR g.valid_until > now())
     AND g.via_membership_id IS NULL AND NOT g.self_scoped
   GROUP BY g.id, g.identity_id, g.role_id
)`

// ReachCoverRow counts who already reaches a place, and who holds some
// other hand-picked list beside it.
type ReachCoverRow struct {
	HaveAll    int32
	HaveBranch int32
	OtherFixed int32
}

// PlaceReachCover counts people whose live access already covers the place:
// no limit on its kind (where no picks means everywhere), or a pick on it or
// on a branch above it. OtherFixed is people with a partial list of its
// siblings: they chose some places here, just not all.
var PlaceReachCover = storm.SQL[ReachCoverRow](reachCTE + `
SELECT
  (SELECT count(DISTINCT g.identity_id) FROM grants g, n
    WHERE g.tenant_id = $1 AND g.revoked_at IS NULL
      AND (g.valid_until IS NULL OR g.valid_until > now()) AND NOT g.self_scoped
      AND NOT EXISTS (SELECT 1 FROM scope_axes ax WHERE ax.tenant_id = $1
                         AND ax.code = n.axis_code AND ax.default_effect = 'deny')
      AND NOT EXISTS (SELECT 1 FROM grant_scopes gs WHERE gs.grant_id = g.id
                         AND gs.axis_code = n.axis_code AND gs.mode = 'include'))::int4 AS have_all,
  (SELECT count(DISTINCT g.identity_id) FROM grants g
     JOIN grant_scopes gs ON gs.grant_id = g.id AND gs.mode = 'include'
     JOIN n ON gs.axis_code = n.axis_code
     JOIN scope_closure c ON c.ancestor_id = gs.scope_node_id AND c.descendant_id = n.id
    WHERE g.tenant_id = $1 AND g.revoked_at IS NULL
      AND (g.valid_until IS NULL OR g.valid_until > now()) AND NOT g.self_scoped
      AND (gs.inherit OR c.depth = 0))::int4 AS have_branch,
  (SELECT count(DISTINCT per.identity_id) FROM per, nsib
    WHERE per.hit > 0 AND per.hit < nsib.c AND NOT per.has_it)::int4 AS other_fixed`)

// ReachGroupRow is people holding one role who might expect a place.
type ReachGroupRow struct {
	RoleID   string
	RoleName string
	People   int32
	Risk     string
}

// PlaceReachMightExpect groups, by role, the people whose picks name every
// sibling of the place one by one but not the place: the lists that looked
// like "all" until it was added.
var PlaceReachMightExpect = storm.SQL[ReachGroupRow](reachCTE + `
SELECT per.role_id::text AS role_id, r.name AS role_name,
       count(DISTINCT per.identity_id)::int4 AS people,
       coalesce((SELECT CASE max(CASE p.risk WHEN 'critical' THEN 3 WHEN 'sensitive' THEN 2 ELSE 1 END)
                        WHEN 3 THEN 'critical' WHEN 2 THEN 'sensitive' ELSE 'normal' END
                   FROM role_permissions_effective rpe
                   JOIN permissions p ON p.id = rpe.permission_id
                  WHERE rpe.role_id = per.role_id), 'normal') AS risk
  FROM per, nsib
  JOIN roles r ON true
 WHERE r.id = per.role_id AND nsib.c > 0 AND per.hit = nsib.c AND NOT per.has_it
 GROUP BY per.role_id, r.name
 ORDER BY count(DISTINCT per.identity_id) DESC, r.name`)

// ReachEntryRow is a group whose entry names every sibling of a place.
type ReachEntryRow struct {
	MembershipID   string
	MembershipName string
	RoleName       string
	Members        int32
}

// PlaceReachGroupEntries finds groups whose live entries name every sibling
// of the place but not the place.
var PlaceReachGroupEntries = storm.SQL[ReachEntryRow](`
WITH n AS (
  SELECT sn.id, sn.axis_code, sn.parent_id FROM scope_nodes sn
   WHERE sn.id::text = $2::text AND sn.tenant_id = $1
), sib AS (
  SELECT s.id FROM scope_nodes s, n
   WHERE s.parent_id = n.parent_id AND s.id <> n.id AND s.status = 'active'
), nsib AS (
  SELECT count(*) AS c FROM sib
), per AS (
  SELECT e.membership_id, e.role_id,
         count(*) FILTER (WHERE mes.scope_node_id IN (SELECT id FROM sib)) AS hit,
         bool_or(mes.scope_node_id = n.id) AS has_it
    FROM membership_entries e
    JOIN membership_entry_scopes mes ON mes.entry_id = e.id AND mes.mode = 'include'
    JOIN n ON mes.axis_code = n.axis_code
   WHERE e.tenant_id = $1 AND e.retired_at IS NULL
   GROUP BY e.id, e.membership_id, e.role_id
)
SELECT m.id::text AS membership_id, m.name AS membership_name, r.name AS role_name,
       (SELECT count(DISTINCT mm.identity_id) FROM membership_members mm
         WHERE mm.membership_id = m.id AND mm.removed_at IS NULL)::int4 AS members
  FROM per, nsib, memberships m, roles r
 WHERE m.id = per.membership_id AND r.id = per.role_id
   AND nsib.c > 0 AND per.hit = nsib.c AND NOT per.has_it
 ORDER BY m.name, r.name`)
