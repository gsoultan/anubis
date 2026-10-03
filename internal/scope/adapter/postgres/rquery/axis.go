package scopermquery

import "github.com/gsoultan/storm"

// UpdateScopeAxis edits an axis's presentation and status.
//
// resolution is deliberately NOT updatable here: it says where a request's
// value for this axis comes from, and changing it reinterprets every grant
// already written against the axis. That is a migration, not an edit.
//
// $7 the tenant: a structure is one tenant's (0056), and so is making it
// strict — which used to deny access in every tenant at once.
var UpdateScopeAxis = storm.SQLExec(`
UPDATE scope_axes
SET display_name = $2, default_effect = $3, status = $4,
    sort_order = $5, ui_schema = $6::jsonb
WHERE code = $1 AND tenant_id = $7`)

// UpdateScopeNodeType edits a level's name and what it may sit under. The
// rules that keep a structure whole (one top level, parents that are levels
// of the same structure, no rule taken from items that rely on it) are the
// trigger's, migration 0055. $1 code, $2 axis, $4 parent level codes, $5 the
// tenant whose level it is.
var UpdateScopeNodeType = storm.SQLExec(`
UPDATE scope_node_types SET display_name = $3, parent_types = $4::text[]
WHERE code = $1 AND axis_code = $2 AND tenant_id = $5`)
