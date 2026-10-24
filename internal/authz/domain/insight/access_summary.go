// Package insight is what the console reads about access as a whole: what a
// person can do in a line, which hand-picked lists name every place there
// is, and who might expect a new place. Read-only facts, computed in SQL.
package insight

import "time"

// AccessSummary is one person's live access in a sentence's worth of facts.
type AccessSummary struct {
	IdentityID string
	Direct     int
	Groups     int
	Roles      []string
	GroupNames []string
	NextEnd    *time.Time
}
