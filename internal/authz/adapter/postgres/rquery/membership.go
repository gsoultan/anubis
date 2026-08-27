package authzrquery

import "github.com/gsoultan/storm"

// MembershipListRow is a membership with its live member count.
type MembershipListRow struct {
	ID          string
	Name        string
	Description string
	MemberCount int32
}

// ListMemberships lists a tenant's memberships with counts in one query.
var ListMemberships = storm.SQL[MembershipListRow](`
SELECT m.id::text AS id, m.name, m.description,
       (SELECT count(*) FROM membership_members mm
         WHERE mm.membership_id = m.id)::int AS member_count
FROM memberships m
WHERE m.tenant_id = $1
ORDER BY m.name`)

// MembershipRow is one membership.
type MembershipRow struct {
	ID          string
	TenantID    string
	Name        string
	Description string
}

// GetMembership fetches one membership within its tenant.
var GetMembership = storm.SQL[MembershipRow](`
SELECT id::text AS id, tenant_id::text AS tenant_id, name, description
FROM memberships
WHERE id = $1 AND tenant_id = $2`)

// CreatedMembershipRow is a fresh membership's id.
type CreatedMembershipRow struct {
	ID string
}

// CreateMembership inserts a membership shell; entries follow separately.
var CreateMembership = storm.SQL[CreatedMembershipRow](`
INSERT INTO memberships (tenant_id, name, description)
VALUES ($1, $2, $3)
RETURNING id::text AS id`)

// EntryRow is one membership entry with its role name.
type EntryRow struct {
	ID           string
	MembershipID string
	RoleID       string
	RoleName     string
}

// ListMembershipEntries fans out over several memberships in ONE query.
var ListMembershipEntries = storm.SQL[EntryRow](`
SELECT me.id::text AS id, me.membership_id::text AS membership_id,
       me.role_id::text AS role_id, r.name AS role_name
FROM membership_entries me
JOIN roles r ON r.id = me.role_id
WHERE me.membership_id = ANY($1::uuid[])
ORDER BY r.name`)

// EntryScopeRow is one entry's scope pin with its node name.
type EntryScopeRow struct {
	EntryID     string
	AxisCode    string
	ScopeNodeID string
	Inherit     bool
	NodeName    string
}

// ListMembershipEntryScopes fans out over several entries in ONE query.
var ListMembershipEntryScopes = storm.SQL[EntryScopeRow](`
SELECT mes.entry_id::text AS entry_id, mes.axis_code,
       mes.scope_node_id::text AS scope_node_id, mes.inherit,
       sn.name AS node_name
FROM membership_entry_scopes mes
JOIN scope_nodes sn ON sn.id = mes.scope_node_id
WHERE mes.entry_id = ANY($1::uuid[])`)

// DeleteMembershipEntries clears a membership's entries before a replace.
var DeleteMembershipEntries = storm.SQLExec(`
DELETE FROM membership_entries WHERE membership_id = $1`)

// InsertedEntryRow is a fresh entry's id.
type InsertedEntryRow struct {
	ID string
}

// InsertMembershipEntry adds one role entry to a membership.
var InsertMembershipEntry = storm.SQL[InsertedEntryRow](`
INSERT INTO membership_entries (membership_id, tenant_id, role_id)
VALUES ($1, $2, $3)
RETURNING id::text AS id`)

// InsertMembershipEntryScope pins one axis of an entry.
var InsertMembershipEntryScope = storm.SQLExec(`
INSERT INTO membership_entry_scopes (entry_id, tenant_id, axis_code,
                                     scope_node_id, inherit)
VALUES ($1, $2, $3, $4, $5)`)

// AssignRow reports how many grants an assignment materialized.
type AssignRow struct {
	GrantsCreated int32
}

// AssignMembership materializes a membership's entries as grants for one
// identity. Semantics live in the membership_assign function.
var AssignMembership = storm.SQL[AssignRow](`
SELECT membership_assign($1, $2, $3) AS grants_created`)

// UnassignRow reports how many grants an unassignment revoked.
type UnassignRow struct {
	GrantsRevoked int32
}

// UnassignMembership revokes the grants a membership materialized.
var UnassignMembership = storm.SQL[UnassignRow](`
SELECT membership_unassign($1, $2) AS grants_revoked`)

// ResyncRow reports how many grants a resync touched.
type ResyncRow struct {
	GrantsChanged int32
}

// ResyncMembership reconciles every member's grants with the current entries.
var ResyncMembership = storm.SQL[ResyncRow](`
SELECT membership_resync($1) AS grants_changed`)
