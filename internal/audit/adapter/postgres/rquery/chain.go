package auditrquery

import "github.com/gsoultan/storm"

// DoneRow carries the result of a void-returning call.
//
// The functions below return void, and a void column is not scannable — while
// storm.SQLExec rightly refuses a statement whose descriptor HAS columns, and
// a SELECT of a function call always has one. `IS NULL` turns void into a
// boolean nobody reads, which is the narrow trick that lets both rules hold.
type DoneRow struct {
	Done bool
}

// AdvisoryLockAuditChain serialises chain appends per tenant for the duration
// of the transaction.
//
// It is per TENANT, not global: the chain is per tenant, so a global lock
// would serialise every tenant's writes behind the busiest one.
// hashtextextended gives a stable 64-bit key from the tenant id, and the
// 'audit:' prefix keeps that key space from colliding with any other advisory
// lock in the system — two features hashing bare uuids would share a namespace
// and deadlock on a coincidence.
var AdvisoryLockAuditChain = storm.SQL[DoneRow](`
SELECT (pg_advisory_xact_lock(hashtextextended('audit:' || $1::text, 0)) IS NULL) AS done`)
