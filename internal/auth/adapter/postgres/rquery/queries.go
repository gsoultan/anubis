// Package authrquery is the ONE place where this context's SQL lives in Go —
// the successor to db/queries/auth/*.sql under ADR-0009 §5: SQL is reviewable
// in exactly one package per context, and every statement here is PREPAREd
// against the live schema at generate time, so a query that drifts from the
// database fails the build naming the column, not the request.
//
// This context is mostly SQL, and that is its shape rather than a shortfall. A
// session and a refresh token are STATE MACHINES: nearly every write is a
// guarded transition stamped with the server's clock —
// `SET status = 'consumed' WHERE status = 'active'` — and the guard has to be
// IN the statement, because a read-then-write is precisely the replay window
// these tables exist to close. A one-time token is consumed by
// DELETE ... RETURNING for the same reason.
//
// Files mirror the old db/queries/auth layout (session, refresh, one_time,
// api_key, signing_key) so review diffs read side by side.
package authrquery

import "github.com/gsoultan/storm"

// Queries is what cmd/stormgen validates and emits scanners for. Every
// declaration in the package MUST be listed: an omitted one does not merely
// lose its generate-time schema check, it REFUSES TO RUN, and the build is
// clean and the tests pass while a scheduled job is where you find out.
func Queries() []storm.RawDecl {
	return []storm.RawDecl{
		// session.go
		CreateSession, GetSessionLive, GetSessionState, ListSessionsByIdentity,
		RevokeSession, RevokeAllSessions, UpdateSessionScopes,
		UpgradeSessionAmr, SetSessionCookieHash, GetSessionByCookieHash,
		// refresh.go
		CreateRefreshToken, ClaimRefreshToken, SetRefreshSuccessor,
		GetRefreshTokenByHash, RevokeRefreshFamily, RevokeRefreshBySession,
		RevokeRefreshBySessions,
		// one_time.go
		ConsumeOneTimeToken, SweepOneTimeTokens,
		// api_key.go
		GetAPIKeyByLookup, ListAPIKeys, RevokeAPIKey,
		// signing_key.go
		SetSigningKeyStatus, PromotePendingKey, DemoteActiveKey,
	}
}
