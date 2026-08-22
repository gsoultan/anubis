// Package domain holds entities and rules. It imports NOTHING outside the Go
// standard library — CI enforces this (ADR-0002 rule 2). The authorization
// engine itself lives in SQL (migrations/0013); what lives here is the
// vocabulary and the invariants the other layers share.
//
// Conventions: one type per file; interfaces stay under 15 methods.
package domain

// Kind classifies an error for transport mapping without the transport
// knowing codes, and without the domain knowing HTTP or connect.
type Kind int

const (
	KindInternal Kind = iota
	KindInvalidArgument
	KindUnauthenticated
	KindPermissionDenied
	KindNotFound
	KindConflict
	KindFailedPrecondition
	KindRateLimited
	KindUnavailable
)
