package authport

import "context"

// OneTimeSweeper removes expired single-use tokens (MFA challenges, auth
// codes, device nonces). They live seconds to minutes; without a sweep the
// table is pure bloat on a path that must stay fast.
type OneTimeSweeper interface {
	SweepExpired(ctx context.Context) (int64, error)
}
