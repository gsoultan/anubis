package grantchange

import "time"

// Record is a change as it was recorded, for the list of recent changes.
type Record struct {
	ID        string
	Kind      string
	Reason    string
	Grants    int
	ActorID   string
	CreatedAt time.Time
	UndoneAt  *time.Time
	// Summary is the Spec as asked, as JSON.
	Summary string
}
