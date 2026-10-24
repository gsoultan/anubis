package authzrquery

import (
	"time"

	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// ChangeCandidateRow is one live grant of a role, as a bulk change sees it.
type ChangeCandidateRow struct {
	ID         string
	IdentityID string
	Username   string
	SelfScoped bool
	FromGroup  bool
	ValidUntil runtime.Null[time.Time]
}

// ChangeCandidates lists the live grants of one role, locked, for a bulk
// change. FOR UPDATE OF g: the plan is worked out from these rows, and two
// changes at once must not both build on the same before. $3 is the cap
// plus one, so the caller can tell "exactly the cap" from "more".
var ChangeCandidates = storm.SQL[ChangeCandidateRow](`
SELECT g.id::text AS id, g.identity_id::text AS identity_id, i.username,
       g.self_scoped, g.via_membership_id IS NOT NULL AS from_group,
       g.valid_until
  FROM grants g
  JOIN identities i ON i.id = g.identity_id
 WHERE g.tenant_id = $1 AND g.role_id::text = $2::text
   AND g.revoked_at IS NULL
   AND (g.valid_until IS NULL OR g.valid_until > now())
 ORDER BY i.username, g.id
 LIMIT $3
   FOR UPDATE OF g`)

// AxisFactsRow is what planning needs to know about one kind of place.
type AxisFactsRow struct {
	Deny   bool
	RootID string
}

// AxisFacts reads whether no picks on an axis means nothing, and its top.
var AxisFacts = storm.SQL[AxisFactsRow](`
SELECT ax.default_effect = 'deny' AS deny, root.id::text AS root_id
  FROM scope_axes ax
  JOIN scope_nodes root ON root.tenant_id = ax.tenant_id
                       AND root.axis_code = ax.code AND root.is_axis_root
 WHERE ax.tenant_id = $1 AND ax.code = $2`)

// NodeFamilyRow is a place's axis, its siblings and everything inside it.
type NodeFamilyRow struct {
	AxisCode string
	Siblings []string
	Below    []string
}

// NodeFamily reads one place's siblings (the other live places under its
// parent) and the places inside it.
var NodeFamily = storm.SQL[NodeFamilyRow](`
SELECT sn.axis_code,
       coalesce((SELECT array_agg(s.id::text ORDER BY s.id) FROM scope_nodes s
                  WHERE s.parent_id = sn.parent_id AND s.id <> sn.id
                    AND s.status = 'active'), '{}')::text[] AS siblings,
       coalesce((SELECT array_agg(c.descendant_id::text) FROM scope_closure c
                  WHERE c.ancestor_id = sn.id AND c.depth > 0), '{}')::text[] AS below
  FROM scope_nodes sn
 WHERE sn.id::text = $2::text AND sn.tenant_id = $1`)

// GrantStateRow is a grant's end date and revoked flag.
type GrantStateRow struct {
	ID         string
	ValidUntil runtime.Null[time.Time]
	Revoked    bool
}

// GrantStates locks and reads the grants an undo would touch.
var GrantStates = storm.SQL[GrantStateRow](`
SELECT id::text AS id, valid_until, revoked_at IS NOT NULL AS revoked
  FROM grants
 WHERE tenant_id = $1 AND id = ANY($2::uuid[])
   FOR UPDATE`)

// CreatedChangeRow is a recorded change's id.
type CreatedChangeRow struct {
	ID string
}

// InsertGrantChange records a bulk change. $5 summary is the request as
// asked, $6 how many grants it altered.
var InsertGrantChange = storm.SQL[CreatedChangeRow](`
INSERT INTO grant_changes (tenant_id, actor_id, kind, reason, summary, grants)
VALUES ($1, $2, $3, $4, $5::jsonb, $6)
RETURNING id::text AS id`)

// DeleteScopesOfGrants clears the picks of grants whose picks are rewritten.
var DeleteScopesOfGrants = storm.SQLExec(`
DELETE FROM grant_scopes WHERE tenant_id = $1 AND grant_id = ANY($2::uuid[])`)

// InsertScopesFromJSON writes picks for many grants in one statement. $2 is
// a JSON array of {grant_id, axis, node, inherit, exclude}.
var InsertScopesFromJSON = storm.SQLExec(`
INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id, inherit, mode)
SELECT x.grant_id, $1, x.axis, x.node, x.inherit,
       CASE WHEN x.exclude THEN 'exclude' ELSE 'include' END
  FROM jsonb_to_recordset($2::jsonb)
       AS x(grant_id uuid, axis text, node uuid, inherit boolean, exclude boolean)`)

// UpdateGrantLives sets end dates and revoked state for many grants. $2 is a
// JSON array of {grant_id, until (unix seconds or null), revoked}. A grant
// revoked by the change gets $3 as its reason; one an undo brings back
// loses its revoke stamp, because the revoke it carried is being undone.
var UpdateGrantLives = storm.SQLExec(`
UPDATE grants g
   SET valid_until = CASE WHEN x.until IS NULL THEN NULL ELSE to_timestamp(x.until) END,
       revoked_at = CASE WHEN x.revoked THEN coalesce(g.revoked_at, now()) ELSE NULL END,
       revoke_reason = CASE WHEN x.revoked THEN coalesce(g.revoke_reason, nullif($3, ''))
                            ELSE NULL END
  FROM jsonb_to_recordset($2::jsonb) AS x(grant_id uuid, until bigint, revoked boolean)
 WHERE g.id = x.grant_id AND g.tenant_id = $1`)

// InsertChangeItems records each grant's before and after. $3 is a JSON
// array of {grant_id, before, after}.
var InsertChangeItems = storm.SQLExec(`
INSERT INTO grant_change_items (change_id, grant_id, tenant_id, before, after)
SELECT $1, x.grant_id, $2, x.before, x.after
  FROM jsonb_to_recordset($3::jsonb) AS x(grant_id uuid, before jsonb, after jsonb)`)

// ChangeItemRow is one grant's recorded before and after.
type ChangeItemRow struct {
	GrantID string
	Before  string
	After   string
}

// ChangeItems reads a change's grants for undo.
var ChangeItems = storm.SQL[ChangeItemRow](`
SELECT grant_id::text AS grant_id, before::text AS before, after::text AS after
  FROM grant_change_items
 WHERE change_id::text = $1::text AND tenant_id = $2`)

// GrantChangeRow is one recorded change.
type GrantChangeRow struct {
	ID        string
	Kind      string
	Reason    string
	Grants    int32
	ActorID   string
	CreatedAt time.Time
	UndoneAt  runtime.Null[time.Time]
	Summary   string
}

// GetGrantChangeForUpdate locks one change, so two undos cannot both run.
var GetGrantChangeForUpdate = storm.SQL[GrantChangeRow](`
SELECT id::text AS id, kind, reason, grants, actor_id::text AS actor_id,
       created_at, undone_at, summary::text AS summary
  FROM grant_changes
 WHERE id::text = $1::text AND tenant_id = $2
   FOR UPDATE`)

// MarkGrantChangeUndone stamps a change as undone.
var MarkGrantChangeUndone = storm.SQLExec(`
UPDATE grant_changes SET undone_at = now(), undone_by = $3
 WHERE id::text = $1::text AND tenant_id = $2 AND undone_at IS NULL`)

// ListGrantChanges is the recent changes in a tenant, newest first.
var ListGrantChanges = storm.SQL[GrantChangeRow](`
SELECT id::text AS id, kind, reason, grants, actor_id::text AS actor_id,
       created_at, undone_at, summary::text AS summary
  FROM grant_changes
 WHERE tenant_id = $1
 ORDER BY created_at DESC
 LIMIT $2`)
