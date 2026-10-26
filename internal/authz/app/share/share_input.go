package authzshare

import "time"

// ShareInput is the request to open one record to one person.
type ShareInput struct {
	RecordKind string
	RecordRef  string
	// RecordScopes is where the record lives — the same targets the
	// application passes to Authorize for it, "_owner" included when the
	// record has one. The sharer is authorised against these for every
	// permission they hand over. "_record" is ignored here on purpose: a
	// share cannot be re-shared, only re-made by somebody who holds the
	// access themselves.
	RecordScopes map[string]string
	ToIdentityID string
	Permissions  []string
	ValidUntil   time.Time
	Note         string
}
