// Package catalogsync is where an application's permission and role catalog
// comes from, and what happened the last time Anubis went and read it.
package catalogsync

import "time"

// Kinds a source can be. One today; the shape is a registry rather than a
// switch so a file drop or an object store is a registration, not a change
// to the sync path.
const (
	KindHTTP = "http"
)

// A source is either on a clock or it is not, and that is the whole
// difference between the scheduled channel and the manual one.
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// Source is a configured origin for ONE application's catalog. The
// application is a pin, not a parameter: whatever the document says, it is
// applied under this application's id and slug and can reach nothing else.
type Source struct {
	ID              string
	TenantID        string
	ApplicationID   string
	ApplicationSlug string
	Kind            string
	Format          string
	Status          string
	Name            string
	// Config is the kind's own settings; only the kind knows what it means.
	// http: {"url": "...", "auth_header": "..."}
	Config []byte
	// IntervalSeconds of 0 is a source that only runs when asked.
	IntervalSeconds int
	LastRunAt       *time.Time
	NextRunAt       *time.Time
	// LastStatus is how the most recent attempt ended, empty when there has
	// never been one.
	LastStatus string
}

// Scheduled reports whether the scheduler will ever pick this source up on
// its own.
func (s Source) Scheduled() bool { return s.IntervalSeconds > 0 }
