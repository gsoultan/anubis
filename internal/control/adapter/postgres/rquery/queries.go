// Package controlrquery is the ONE place where this context's SQL lives in Go
// — the successor to db/queries/control/*.sql under ADR-0009 §5: SQL is
// reviewable in exactly one package per context, and every statement here is
// PREPAREd against the live schema at generate time, so a query that drifts
// from the database fails the build naming the column, not the request.
//
// What is here, and why it is not a builder:
//
//   - The JOINS. Reading a key or an assignment alongside its owner's status
//     is one query on purpose — checking the owner separately is a second
//     round trip AND a window where the two disagree.
//   - The guarded UPDATES that set a SERVER-side expression. `revoked_at =
//     now()` on a client clock is not the same fact: app servers skew, and a
//     revocation that appears to precede the token it revokes is unreadable
//     afterwards. `token_epoch = token_epoch + 1` is stronger still — computed
//     in Go it is a read-modify-write, and two concurrent disables would lose
//     one increment and leave one operator's tokens live.
//
// storm has no way to SET a column to an expression (its Mut takes values), so
// these stay SQL. That is the correct home for them either way: they are three
// lines of SQL that say exactly what they mean.
package controlrquery

import "github.com/gsoultan/storm"

// Queries is what cmd/stormgen validates and emits scanners for. Every
// declaration in the package MUST be listed: an omitted one does not merely
// lose its generate-time schema check, it REFUSES TO RUN, and the build is
// clean and the tests pass while a scheduled job is where you find out.
func Queries() []storm.RawDecl {
	return []storm.RawDecl{
		// api_key.go
		PlatformAPIKeyByLookup, ListPlatformAPIKeys,
		TouchPlatformAPIKeyUsed, RevokePlatformAPIKey,
		// assignment.go
		ListAssignments, ListAssignmentsForOperator, RevokeAssignment, HasAnyPlatformOwner,
		// user.go
		GetPlatformUserByUsername, ListPlatformUsers,
		SetPlatformUserStatus, TouchPlatformUserLogin,
		StageTotpSecret, ConfirmTotpEnrolment, AdvanceTotpStep, ClearTotp,
		// refresh.go
		InsertPlatformRefreshRoot, PlatformRefreshByHash, ConsumePlatformRefresh,
		RevokePlatformRefreshFamily, SweepPlatformRefresh,
	}
}
