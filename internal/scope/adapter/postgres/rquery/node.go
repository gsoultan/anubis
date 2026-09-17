package scopermquery

import (
	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/runtime"
)

// NodeRow is one scope node with the count the expand affordance is drawn
// from.
//
// ChildCount counts the children THIS CALL would return — same tenant, same
// include_archived. A count that disagreed with the listing would draw a
// chevron that expands to nothing, or hide one that had children behind it.
type NodeRow struct {
	ID          string
	TenantID    string
	ParentID    runtime.Null[string]
	IsAxisRoot  bool
	Status      string
	AxisCode    string
	NodeType    string
	Slug        string
	Name        string
	ExternalRef runtime.Null[string]
	ChildCount  int32
}

const nodeCols = `
       n.id::text AS id, n.tenant_id::text AS tenant_id,
       n.parent_id::text AS parent_id, n.is_axis_root, n.status, n.axis_code,
       n.node_type, n.slug, n.name, n.external_ref`

// ListScopeNodes is KEYSET paged, ordered by (name, id).
//
// Not OFFSET: at a million nodes OFFSET re-scans everything it skips, and it
// drops or repeats rows when a sync inserts ahead of the cursor. name is not
// unique, which is why id is in both the ORDER BY and the comparison.
// Index: scope_nodes_paging (0039).
//
// The child count costs a per-row index scan on scope_nodes_sibling_slug:
// measured 0.154 ms -> 0.393 ms for a 200-row page of the 20k-node customer
// axis, which is the price of a chevron that tells the truth.
var ListScopeNodes = storm.SQL[NodeRow](`
SELECT` + nodeCols + `,
       (SELECT count(*) FROM scope_nodes c
         WHERE c.parent_id = n.id
           AND ($6::boolean OR c.status = 'active'))::int AS child_count
FROM scope_nodes n
WHERE n.tenant_id = $1
  AND n.axis_code = $2
  AND ($3::uuid IS NULL OR n.parent_id = $3)
  AND ($4::text IS NULL OR n.name ILIKE '%' || $4 || '%')
  AND ($6::boolean OR n.status = 'active')
  AND ($5::text IS NULL
       OR n.name > $5::text
       OR (n.name = $5::text AND n.id > $7::uuid))
ORDER BY n.name, n.id
LIMIT $8`)

const activeChildCount = `
       (SELECT count(*) FROM scope_nodes c
         WHERE c.parent_id = n.id AND c.status = 'active')::int AS child_count`

var GetScopeNode = storm.SQL[NodeRow](`
SELECT` + nodeCols + `,` + activeChildCount + `
FROM scope_nodes n
WHERE n.id = $1 AND n.tenant_id = $2`)

var GetScopeNodeByRef = storm.SQL[NodeRow](`
SELECT` + nodeCols + `,` + activeChildCount + `
FROM scope_nodes n
WHERE n.tenant_id = $1 AND n.axis_code = $2 AND n.external_ref = $3`)

// ScopeNodesByIDs resolves a HANDFUL of nodes by id — the names beside the
// grants on one screen. The console used to pull every node in every axis to
// render a dozen labels.
var ScopeNodesByIDs = storm.SQL[NodeRow](`
SELECT` + nodeCols + `,` + activeChildCount + `
FROM scope_nodes n
WHERE n.tenant_id = $1 AND n.id = ANY($2::uuid[])`)

// AncestorRow is one step of the chain from an axis root down to a node.
type AncestorRow struct {
	ID          string
	AxisCode    string
	NodeType    string
	ParentID    runtime.Null[string]
	Slug        string
	Name        string
	ExternalRef runtime.Null[string]
	Status      string
	IsAxisRoot  bool
	Depth       int16
}

// ScopeAncestors is what makes a scope decision explainable: "this grant
// reaches here BECAUSE it was given on that ancestor".
//
// Read from the closure table, so it costs one index scan rather than a
// recursive walk.
var ScopeAncestors = storm.SQL[AncestorRow](`
SELECT n.id::text AS id, n.axis_code, n.node_type, n.parent_id::text AS parent_id,
       n.slug, n.name, n.external_ref, n.status, n.is_axis_root, c.depth
  FROM scope_closure c
  JOIN scope_nodes n ON n.id = c.ancestor_id
 WHERE c.descendant_id = $1 AND n.tenant_id = $2
 ORDER BY c.depth DESC`)

// ArchiveScopeNode retires a node. The axis root is exempt: archiving it would
// leave the axis with no tree to hang anything from.
var ArchiveScopeNode = storm.SQLExec(`
UPDATE scope_nodes SET status = 'archived', updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND NOT is_axis_root`)

// RenameScopeNode also REACTIVATES, because a rename is how an operator
// un-archives: the node came back in the feed under a new name.
var RenameScopeNode = storm.SQLExec(`
UPDATE scope_nodes SET name = $3, status = 'active', updated_at = now()
WHERE id = $1 AND tenant_id = $2`)
