-- name: ListMemberships :many
SELECT m.id, m.name, m.description,
       (SELECT count(*) FROM membership_members mm
         WHERE mm.membership_id = m.id)::int AS member_count
FROM memberships m
WHERE m.tenant_id = sqlc.arg(tenant_id)
ORDER BY m.name;

-- name: GetMembership :one
SELECT id, tenant_id, name, description
FROM memberships
WHERE id = sqlc.arg(id) AND tenant_id = sqlc.arg(tenant_id);

-- name: CreateMembership :one
INSERT INTO memberships (tenant_id, name, description)
VALUES (sqlc.arg(tenant_id), sqlc.arg(name), sqlc.arg(description))
RETURNING id;

-- name: ListMembershipEntries :many
SELECT me.id, me.membership_id, me.role_id, r.name AS role_name
FROM membership_entries me
JOIN roles r ON r.id = me.role_id
WHERE me.membership_id = ANY(sqlc.arg(membership_ids)::uuid[])
ORDER BY r.name;

-- name: ListMembershipEntryScopes :many
SELECT mes.entry_id, mes.axis_code, mes.scope_node_id, mes.inherit,
       sn.name AS node_name
FROM membership_entry_scopes mes
JOIN scope_nodes sn ON sn.id = mes.scope_node_id
WHERE mes.entry_id = ANY(sqlc.arg(entry_ids)::uuid[]);

-- name: DeleteMembershipEntries :exec
DELETE FROM membership_entries WHERE membership_id = sqlc.arg(membership_id);

-- name: InsertMembershipEntry :one
INSERT INTO membership_entries (membership_id, tenant_id, role_id)
VALUES (sqlc.arg(membership_id), sqlc.arg(tenant_id), sqlc.arg(role_id))
RETURNING id;

-- name: InsertMembershipEntryScope :exec
INSERT INTO membership_entry_scopes (entry_id, tenant_id, axis_code,
                                     scope_node_id, inherit)
VALUES (sqlc.arg(entry_id), sqlc.arg(tenant_id), sqlc.arg(axis_code),
        sqlc.arg(scope_node_id), sqlc.arg(inherit));

-- name: AssignMembership :one
SELECT membership_assign(sqlc.arg(identity_id), sqlc.arg(membership_id),
                         sqlc.arg(assigned_by)) AS grants_created;

-- name: UnassignMembership :one
SELECT membership_unassign(sqlc.arg(identity_id), sqlc.arg(membership_id)) AS grants_revoked;

-- name: ResyncMembership :one
SELECT membership_resync(sqlc.arg(membership_id)) AS grants_changed;
