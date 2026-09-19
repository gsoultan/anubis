// Package scopermquery is the ONE place where this context's SQL lives in Go —
// the successor to db/queries/scope/*.sql under ADR-0009 §5: SQL is reviewable
// in exactly one package per context, and every statement here is PREPAREd
// against the live schema at generate time, so a query that drifts from the
// database fails the build naming the column, not the request.
//
// This context is mostly SQL, and that is the right shape for it rather than a
// concession. The tree's writes are database FUNCTIONS — scope_add_node,
// scope_move_node, scope_ensure_root, scope_sync_apply — because a move has to
// rewrite the closure table and re-check the type rules in one statement, and
// doing that from Go would be several round trips with the invariant open
// between them. What the model buys here is the reads that are genuinely reads.
package scopermquery

import "github.com/gsoultan/storm"

// Queries is what cmd/stormgen validates and emits scanners for. Every
// declaration in the package MUST be listed: an omitted one does not merely
// lose its generate-time schema check, it REFUSES TO RUN, and the build is
// clean and the tests pass while a scheduled job is where you find out.
func Queries() []storm.RawDecl {
	return []storm.RawDecl{
		// node.go
		ListScopeNodes, GetScopeNode, GetScopeNodeByRef, ScopeNodesByIDs,
		ScopeAncestors, ArchiveScopeNode, RenameScopeNode,
		// tree.go
		EnsureAxisRoot, AddScopeNode, MoveScopeNode, ScopeSyncApply,
		// axis.go
		UpdateScopeAxis,
		// sync.go
		UpdateSyncSource, CreateSyncSource, SetSyncSchedule,
		RecordSyncFailure, ScheduleNextSyncSource, ListSyncRuns,
	}
}
