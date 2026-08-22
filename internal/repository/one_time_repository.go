package repository

import (
	"context"
	"time"
)

// OneTimeRepository stores single-use state (MFA tokens, PKCE codes, device
// nonces). ConsumeOneTime atomically deletes and returns the payload; a
// second call with the same hash returns domain.ErrNotFound.
type OneTimeRepository interface {
	CreateOneTime(ctx context.Context, tenantID, kind string, hash []byte, payload []byte, expiresAt time.Time) (string, error)
	ConsumeOneTime(ctx context.Context, kind string, hash []byte) (tenantID string, payload []byte, err error)
}
