package identityapp

import "context"

// RetentionUsecase enforces data-retention policy. Separate from the admin
// surface because it runs unattended: the scheduler calls it, not a person.
type RetentionUsecase interface {
	// Sweep stamps deadlines from realm policy, anonymises everything past
	// its deadline, and shreds those identities' PII keys.
	Sweep(ctx context.Context) (SweepReport, error)
	// Erase executes an approved right-to-erasure request immediately.
	Erase(ctx context.Context, tenantID, identityID, reason string) error
}
