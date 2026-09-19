package scopermquery

import "github.com/gsoultan/storm"

// The tree's writes are database functions, and deliberately.
//
// A move rewrites the closure table for the whole moved subtree and re-checks
// the parent-type rules; an add does the same for one row. Doing either from
// Go is several round trips with the invariant open between them, and the
// closure table is what authorize() reads — so a half-applied move is a
// wrong ANSWER, not a slow one.

// NodeIDRow is a node id returned by one of the tree functions.
type NodeIDRow struct {
	NodeID string
}

// EnsureAxisRoot creates a tenant's root for an axis if it has none, and
// returns it either way — the idempotence is the point: every path that needs
// a tree can call it without checking first.
var EnsureAxisRoot = storm.SQL[NodeIDRow](`
SELECT scope_ensure_root($1, $2)::text AS node_id`)

// AddScopeNode inserts one node, with the type rules and closure maintenance
// the function owns.
//
// nullif on external_ref: ” is a node with no upstream identity, and NULL is
// what the partial unique index on (tenant, axis, external_ref) expects — an
// empty string would make every unsynced node collide with every other.
var AddScopeNode = storm.SQL[NodeIDRow](`
SELECT scope_add_node($1, $2, $3, $4, $5, $6, nullif($7, ''))::text AS node_id`)

// DoneRow carries the result of a void-returning call. `IS NULL` turns void
// into a scannable boolean, because storm.SQLExec rightly refuses a statement
// whose descriptor has columns and a SELECT of a function call always has one.
type DoneRow struct {
	Done bool
}

// MoveScopeNode re-parents a node and rewrites its subtree's closure rows.
var MoveScopeNode = storm.SQL[DoneRow](`
SELECT (scope_move_node($1, $2) IS NULL) AS done`)

// ReportRow is a sync run's report as JSON text.
type ReportRow struct {
	Report string
}

// ScopeSyncApply reconciles a whole feed in one statement.
//
// The function opens its own run row and catches per-row errors, so a feed
// with three bad rows records three errors and applies the rest — which is
// what an operator needs from a nightly import, rather than an all-or-nothing
// that fails on somebody's typo.
var ScopeSyncApply = storm.SQL[ReportRow](`
SELECT scope_sync_apply($1, $2::jsonb, $3)::text AS report`)
