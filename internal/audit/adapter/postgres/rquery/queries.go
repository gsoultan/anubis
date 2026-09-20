// Package auditrquery is the ONE place where this context's SQL lives in Go —
// the successor to db/queries/audit/*.sql under ADR-0009 §5: SQL is reviewable
// in exactly one package per context, and every statement here is PREPAREd
// against the live schema at generate time, so a query that drifts from the
// database fails the build naming the column, not the request.
//
// Most of the audit context does NOT live here. The chain append, the chain
// walk and the log search are ordinary reads and writes of one table, and
// those are generated builders in rgen — typed, and dynamic without being
// assembled from strings. What is left is the residue a builder cannot state:
// two void-returning functions, a session lock, and two aggregates using
// FILTER. Each is SQL because it is genuinely SQL, not because converting it
// was inconvenient.
package auditrquery

import "github.com/gsoultan/storm"

// Queries is what cmd/stormgen validates and emits scanners for. Every
// declaration in the package MUST be listed: an omitted one does not merely
// lose its generate-time schema check, it REFUSES TO RUN, and the build is
// clean and the tests pass while a scheduled job is where you find out.
func Queries() []storm.RawDecl {
	return []storm.RawDecl{
		InsertAuditAnchor, AuditAnchorsFrom,
		// chain.go
		AdvisoryLockAuditChain,
		// partitions.go
		EnsureAuditPartitions, EnsureRefreshPartitions,
		// dashboard.go
		CountDecisions24h, ReuseSignal,
	}
}
