// Package platformrquery holds the technical context's SQL.
//
// Two statements, both advisory locks, and neither is a table read — there is
// no model here and there never will be. They are SESSION-scoped, so they must
// run on one pinned connection (database.Executor maps *pgxpool.Conn to
// storm's connection adapter for exactly this): pg_advisory_lock is held by
// the connection that took it, and releasing through a pool releases on
// whichever connection came back next, which releases nothing and leaks the
// lock until that connection is recycled.
package platformrquery

import "github.com/gsoultan/storm"

// Queries is what cmd/stormgen validates and emits scanners for.
func Queries() []storm.RawDecl {
	return []storm.RawDecl{TryAdvisoryLock, AdvisoryUnlock}
}

// AcquiredRow answers pg_try_advisory_lock.
type AcquiredRow struct {
	Acquired bool
}

// TryAdvisoryLock takes a session lock, or reports that somebody else has it.
//
// try, not the blocking form: a maintenance job that cannot get the lock has
// nothing to wait for — another replica is already doing the work.
var TryAdvisoryLock = storm.SQL[AcquiredRow](`
SELECT pg_try_advisory_lock($1) AS acquired`)

// ReleasedRow answers pg_advisory_unlock. False means this session did not
// hold the lock, which is a bug in the caller rather than a race.
type ReleasedRow struct {
	Released bool
}

var AdvisoryUnlock = storm.SQL[ReleasedRow](`
SELECT pg_advisory_unlock($1) AS released`)
