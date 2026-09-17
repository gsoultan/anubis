// Package gaterquery is the ONE place where this context's SQL lives in Go —
// the successor to db/queries/gate/*.sql under ADR-0009 §5: SQL is reviewable
// in exactly one package per context, and every statement here is PREPAREd
// against the live schema at generate time, so a query that drifts from the
// database fails the build naming the column, not the request.
//
// This context declares NO MODELS, and that is what it is: the gate owns no
// table. It reads eight of them — scope, authz, identity, tenancy — to freeze
// a snapshot, and modelling another context's tables here would put their
// shape in this package's hands, which is the boundary AGENTS.md draws.
//
// ALL of these must run inside ONE REPEATABLE READ read-only transaction
// (ADR-0005 §10). Loading eight tables in separate snapshots yields a torn
// read — a grant referencing a scope node absent from the node map — which is
// a silent wrong ANSWER, roughly weekly, and unreproducible.
// internal/snapshot.Loader owns that transaction and asserts the isolation.
package gaterquery

import "github.com/gsoultan/storm"

// Queries is what cmd/stormgen validates and emits scanners for. Every
// declaration in the package MUST be listed: an omitted one does not merely
// lose its generate-time schema check, it REFUSES TO RUN, and the build is
// clean and the tests pass while a scheduled job is where you find out.
func Queries() []storm.RawDecl {
	return []storm.RawDecl{
		SnapshotAxes, SnapshotNodes, SnapshotGrants, SnapshotGrantScopes,
		SnapshotRolePermissions, SnapshotPermissions, SnapshotIdentities,
		SnapshotRoutes, SnapshotTenants, SnapshotCatalogVersion,
		SnapshotRevokedSessions,
	}
}
