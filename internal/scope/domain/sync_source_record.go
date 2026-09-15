package scopedomain

import "time"

// MinSyncIntervalSeconds is the floor the schema enforces and the app tier
// repeats, so a bad interval is refused with a message instead of a constraint
// violation. A structure is an org chart or a customer list: it changes when
// HR or sales changes it. Anything faster is Anubis hammering somebody else's
// ERP for rows that did not move.
const MinSyncIntervalSeconds = 300

type SyncSourceRecord struct {
	ID string
	// TenantID is carried on the record, not taken from an ambient principal.
	// Every operator-facing path already scopes by the caller's tenant; the
	// scheduler has no caller, so the tenant it acts for has to be a value it
	// is holding rather than one it assumes.
	TenantID  string
	Axis      string
	Kind      string
	Status    string
	Config    []byte
	LastRunAt *time.Time
	// IntervalSeconds is 0 for a source that only runs when somebody asks.
	IntervalSeconds int32
	// NextRunAt is nil for the same reason. A manual source and a scheduled
	// one differ by these two fields and nothing else.
	NextRunAt *time.Time
}

// Scheduled reports whether a timer owns this source. The scheduler asks
// before rescheduling: a manual run must not plant a next_run_at on a source
// nobody asked to automate.
func (s SyncSourceRecord) Scheduled() bool {
	return s.IntervalSeconds > 0
}
