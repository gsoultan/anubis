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
//   - The GUARDED updates. `SET revoked_at = now() WHERE family_id = $1 AND
//     revoked_at IS NULL` addresses rows by something other than the primary
//     key, and the guard has to be in the statement: a read-then-write is the
//     window the guard exists to close.
//
// The server-side expressions themselves are no longer the reason. storm can
// say `revoked_at = now()` and `token_epoch = token_epoch + 1` since v0.16.0
// (SetRevokedAtNow, IncTokenEpoch), and it still matters that the DATABASE
// computes them — a client clock makes a revocation that appears to precede
// the token it revoked, and an increment computed in Go is a read-modify-write
// where two concurrent disables lose one and leave an operator's tokens live.
// What keeps these in SQL is the WHERE, not the SET.
//
// The single-row form went the other way: a lookup by lower(username) is
// platformuser.Username.EqLower now, because EqLower lowers to the expression
// the unique index is built on.
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
		ListPlatformUsers,
		SetPlatformUserStatus, TouchPlatformUserLogin,
		StageTotpSecret, ConfirmTotpEnrolment, AdvanceTotpStep, ClearTotp,
		// refresh.go
		InsertPlatformRefreshRoot, PlatformRefreshByHash, ConsumePlatformRefresh,
		RevokePlatformRefreshFamily, SweepPlatformRefresh,
	}
}
