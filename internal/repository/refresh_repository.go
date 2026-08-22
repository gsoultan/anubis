package repository

import (
	"context"
	"time"
)

// RefreshRepository implements single-use rotation with theft detection.
type RefreshRepository interface {
	CreateRefresh(ctx context.Context, in RefreshInput) (string, error)
	ClaimRefresh(ctx context.Context, hash []byte) (*RefreshClaim, error)
	RefreshByHash(ctx context.Context, hash []byte) (*RefreshInfo, error)
	SetRefreshSuccessor(ctx context.Context, id string, expiresAt time.Time, successorID string) error
	RevokeRefreshFamily(ctx context.Context, familyID string) (int64, error)
	RevokeRefreshBySessions(ctx context.Context, sessionIDs []string) (int64, error)
}
