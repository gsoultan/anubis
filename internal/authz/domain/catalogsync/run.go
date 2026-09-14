package catalogsync

import "time"

// What a run came to. 'skipped' is not a failure and not an apply: the
// document was byte-identical to the one already installed, so nothing was
// written and no manifest version was burned.
const (
	RunRunning = "running"
	RunOK      = "ok"
	RunFailed  = "failed"
	RunDry     = "dry_run"
	RunSkipped = "skipped"
)

// Actors a run can have. A scheduled run has no person behind it and must
// not borrow one — an audit trail that credits a human for what a timer did
// is worse than no trail.
const (
	ActorSystem = "system"
)

// Run is one attempt to read a source and apply what it returned.
type Run struct {
	ID          string
	SourceID    string
	StartedAt   time.Time
	FinishedAt  *time.Time
	Dry         bool
	Status      string
	Actor       string
	DocumentSHA string
	// Report is the apply report as JSON, empty when there was not one.
	Report string
	Error  string
}
