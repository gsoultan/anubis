package auditdomain

import "time"

type AuditQuery struct {
	ActorID string
	// Action is a case-insensitive substring; empty matches any.
	Action string
	// Result is an exact match on allow/deny/error; empty matches any.
	Result string
	From   *time.Time
	To     *time.Time
	// BeforeSeq is the keyset cursor the repository reads. The interactor
	// derives it from PageToken so the token's format stays in one place.
	BeforeSeq *int64
	// PageToken is the opaque next_page_token from a previous response.
	PageToken string
	Limit     int
}
