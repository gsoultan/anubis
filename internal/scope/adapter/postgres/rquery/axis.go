package scopermquery

import "github.com/gsoultan/storm"

// UpdateScopeAxis edits an axis's presentation and status.
//
// resolution is deliberately NOT updatable here: it says where a request's
// value for this axis comes from, and changing it reinterprets every grant
// already written against the axis. That is a migration, not an edit.
var UpdateScopeAxis = storm.SQLExec(`
UPDATE scope_axes
SET display_name = $2, default_effect = $3, status = $4,
    sort_order = $5, ui_schema = $6::jsonb
WHERE code = $1`)
