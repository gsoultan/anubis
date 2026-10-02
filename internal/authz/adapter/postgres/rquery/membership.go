package authzrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// MembershipListRow is a membership with how many people hold it now.
type MembershipListRow struct {
	ID          string
	Name        string
	Description string
	AnchorAxis  runtime.Null[string]
	MemberCount int32
}

// ListMemberships lists a tenant's memberships with counts in one query.
//
// The count is PEOPLE with a current assignment. One person can hold a
// where-assigned membership at several places, and a lapsed or removed
// assignment is history, not a member.
var ListMemberships = storm.SQL[MembershipListRow](`
SELECT m.id::text AS id, m.name, m.description, m.anchor_axis,
       (SELECT count(DISTINCT mm.identity_id) FROM membership_members mm
         WHERE mm.membership_id = m.id AND mm.removed_at IS NULL
           AND (mm.valid_until IS NULL OR mm.valid_until > now()))::int AS member_count
FROM memberships m
WHERE m.tenant_id = $1
ORDER BY m.name`)

// MembershipRow is one membership.
type MembershipRow struct {
	ID          string
	TenantID    string
	Name        string
	Description string
	AnchorAxis  runtime.Null[string]
}

// GetMembership fetches one membership within its tenant — which is how every
// membership write proves the id it was handed is the caller's to touch.
var GetMembership = storm.SQL[MembershipRow](`
SELECT id::text AS id, tenant_id::text AS tenant_id, name, description, anchor_axis
FROM memberships
WHERE id = $1 AND tenant_id = $2`)

// CreatedMembershipRow is a fresh membership's id.
type CreatedMembershipRow struct {
	ID string
}

// CreateMembership inserts a membership shell; entries follow separately.
// $4 anchor axis, empty for the same places for everyone.
var CreateMembership = storm.SQL[CreatedMembershipRow](`
INSERT INTO memberships (tenant_id, name, description, anchor_axis)
VALUES ($1, $2, $3, NULLIF($4, ''))
RETURNING id::text AS id`)

// EntryRow is one membership entry with its role name.
type EntryRow struct {
	ID           string
	MembershipID string
	RoleID       string
	RoleName     string
}

// ListMembershipEntries fans out over several memberships in ONE query. Live
// entries only: a retired one is history its grants still point at.
var ListMembershipEntries = storm.SQL[EntryRow](`
SELECT me.id::text AS id, me.membership_id::text AS membership_id,
       me.role_id::text AS role_id, r.name AS role_name
FROM membership_entries me
JOIN roles r ON r.id = me.role_id
WHERE me.membership_id = ANY($1::uuid[]) AND me.retired_at IS NULL
ORDER BY r.name`)

// EntryScopeRow is one entry's scope pin with its node name.
type EntryScopeRow struct {
	EntryID     string
	AxisCode    string
	ScopeNodeID string
	Inherit     bool
	Exclude     bool
	NodeName    string
}

// ListMembershipEntryScopes fans out over several entries in ONE query.
var ListMembershipEntryScopes = storm.SQL[EntryScopeRow](`
SELECT mes.entry_id::text AS entry_id, mes.axis_code,
       mes.scope_node_id::text AS scope_node_id, mes.inherit,
       mes.mode = 'exclude' AS exclude,
       sn.name AS node_name
FROM membership_entry_scopes mes
JOIN scope_nodes sn ON sn.id = mes.scope_node_id
WHERE mes.entry_id = ANY($1::uuid[])
ORDER BY mes.entry_id, mes.axis_code, mes.mode DESC, sn.name`)

// RetireMembershipEntries takes entries out of a membership without deleting
// them (0054). Deleting was refused by the grants that point back at an entry
// — revoked ones included, since grants are kept as history — so the first
// person to ever hold a membership froze its contents. $2 the entry ids.
var RetireMembershipEntries = storm.SQLExec(`
UPDATE membership_entries SET retired_at = now()
 WHERE membership_id = $1 AND id = ANY($2::uuid[]) AND retired_at IS NULL`)

// InsertedEntryRow is a fresh entry's id.
type InsertedEntryRow struct {
	ID string
}

// InsertMembershipEntry adds one role entry to a membership.
var InsertMembershipEntry = storm.SQL[InsertedEntryRow](`
INSERT INTO membership_entries (membership_id, tenant_id, role_id)
VALUES ($1, $2, $3)
RETURNING id::text AS id`)

// InsertMembershipEntryScope pins one axis of an entry. $6 exclude.
//
// membership_materialize (0054) copies mode onto every grant it creates, so an
// entry's carve-out reaches the grants rather than being quietly dropped into
// a wider grant than the entry described.
var InsertMembershipEntryScope = storm.SQLExec(`
INSERT INTO membership_entry_scopes (entry_id, tenant_id, axis_code,
                                     scope_node_id, inherit, mode)
VALUES ($1, $2, $3, $4, $5, CASE WHEN $6::boolean THEN 'exclude' ELSE 'include' END)`)

// AssignRow is the assignment an assign created — empty when the person
// already held the membership at that place.
type AssignRow struct {
	AssignmentID string
}

// AssignMembership materialises a membership for one person, at one place
// when it applies where assigned. Semantics live in membership_assign.
// $1 identity, $2 membership, $3 by, $4 place (empty for none), $5 inherit,
// $6 valid_until (NULL for open-ended), $7 reason.
//
// It returns the assignment only. Counting its grants in the same statement
// reads the statement's own snapshot, taken before the function inserted
// them, and so always said 0 — CountAssignmentGrants asks afterwards.
var AssignMembership = storm.SQL[AssignRow](`
SELECT COALESCE(membership_assign($1::uuid, $2::uuid, $3::uuid, NULLIF($4::text, '')::uuid,
                                  $5::boolean, $6::timestamptz, $7::text)::text, '') AS assignment_id`)

// CountAssignmentGrants is how many live grants one assignment gives.
var CountAssignmentGrants = storm.SQL[CountRow](`
SELECT count(*) AS count FROM grants WHERE via_member_id = $1::uuid AND revoked_at IS NULL`)

// UnassignRow reports how many grants a removal revoked.
type UnassignRow struct {
	GrantsRevoked int32
}

// UnassignMembership takes a person out of a membership at every place.
// $3 by, $4 reason (kept on each revoked grant).
var UnassignMembership = storm.SQL[UnassignRow](`
SELECT membership_unassign($1::uuid, $2::uuid, $3::uuid, $4::text) AS grants_revoked`)

// LeaveMembership ends ONE assignment, in the caller's tenant: the same person
// may hold the same membership at other places, and those stay. No row means
// no such current assignment here. $3 by, $4 reason.
var LeaveMembership = storm.SQL[UnassignRow](`
SELECT membership_leave(mm.id, $3::uuid, $4::text) AS grants_revoked
  FROM membership_members mm
 WHERE mm.id = NULLIF($1::text, '')::uuid AND mm.tenant_id = $2::uuid AND mm.removed_at IS NULL`)

// ResyncRow reports how many grants a resync touched.
type ResyncRow struct {
	GrantsChanged int32
}

// ResyncMembership reconciles every member's grants with the current entries.
var ResyncMembership = storm.SQL[ResyncRow](`
SELECT membership_resync($1) AS grants_changed`)

// AssignmentRow is one current assignment, with the names a screen shows.
type AssignmentRow struct {
	ID             string
	MembershipID   string
	MembershipName string
	AnchorAxis     runtime.Null[string]
	IdentityID     string
	Username       string
	NodeID         runtime.Null[string]
	NodeName       runtime.Null[string]
	Inherit        bool
	ValidUntil     runtime.Null[time.Time]
	Reason         runtime.Null[string]
	AssignedAt     time.Time
	AssignedBy     string
}

// ListMembershipAssignments lists current assignments — everybody in one
// membership, every membership one person holds, or both. Always inside the
// caller's tenant ($1): an id from somewhere else matches nothing. Newest
// first, keyset-paged on (assigned_at, id) because a membership can hold
// thousands. $2 membership, $3 identity (either may be empty), $4 cursor,
// $5 limit.
var ListMembershipAssignments = storm.SQL[AssignmentRow](`
SELECT mm.id::text AS id, mm.membership_id::text AS membership_id,
       m.name AS membership_name, m.anchor_axis,
       mm.identity_id::text AS identity_id, i.username,
       mm.scope_node_id::text AS node_id, sn.name AS node_name, mm.inherit,
       mm.valid_until, mm.reason, mm.assigned_at, mm.assigned_by::text AS assigned_by
  FROM membership_members mm
  JOIN memberships m      ON m.id = mm.membership_id
  JOIN identities i       ON i.id = mm.identity_id
  LEFT JOIN scope_nodes sn ON sn.id = mm.scope_node_id
 WHERE mm.tenant_id = $1 AND mm.removed_at IS NULL
   AND ($2::text = '' OR mm.membership_id = NULLIF($2::text, '')::uuid)
   AND ($3::text = '' OR mm.identity_id = NULLIF($3::text, '')::uuid)
   AND ($4::text = ''
        OR (mm.assigned_at, mm.id) < (
             SELECT a.assigned_at, a.id FROM membership_members a
              WHERE a.id = NULLIF($4::text, '')::uuid))
 ORDER BY mm.assigned_at DESC, mm.id DESC
 LIMIT $5`)
