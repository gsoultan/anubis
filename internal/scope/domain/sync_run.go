package scopedomain

import "time"

// SyncRun is one reconciliation of a structure feed, as the database has
// recorded it since migration 0017. Dry runs are kept alongside real ones:
// "we checked on Tuesday and it would have archived four thousand nodes" is
// exactly the evidence somebody wants, and the run that did NOT happen is
// otherwise invisible.
type SyncRun struct {
	ID       string
	SourceID string
	AxisCode string
	StartedAt time.Time
	// FinishedAt is nil while a run is in flight, or if the process died
	// mid-run — a row stuck at 'running' is itself the diagnosis.
	FinishedAt *time.Time
	Dry        bool
	// Status: running | ok | failed | dry_run.
	Status string
	// Report is the reconciler's own JSON, passed through rather than
	// re-modelled: added/renamed/moved/archived/unchanged plus the rows it
	// could not place.
	Report []byte
}
