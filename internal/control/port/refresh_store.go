package controlport

import (
	"context"
	"time"

	controldomain "github.com/gsoultan/anubis/internal/control/domain"
)

// PlatformRefreshStore persists operator refresh chains (migration 0031).
// Only hashes cross this port; the secret never exists in storable form.
type PlatformRefreshStore interface {
	// CreateRefreshFamily begins a sign-in: the returned family id is what
	// revocation and every successor reference.
	CreateRefreshFamily(ctx context.Context, platformUserID string, tokenHash []byte, expiresAt time.Time) (string, error)
	// AppendRefresh adds the successor minted by a rotation.
	AppendRefresh(ctx context.Context, platformUserID, familyID string, tokenHash []byte, expiresAt time.Time) error
	// RefreshByHash returns nil when nothing matches — indistinguishable
	// from expired on purpose; both answer invalid_refresh_token.
	RefreshByHash(ctx context.Context, tokenHash []byte) (*controldomain.PlatformRefresh, error)
	// ConsumeRefresh marks one token used, atomically. false means somebody
	// else consumed it first — the caller must treat that as reuse.
	ConsumeRefresh(ctx context.Context, id string) (bool, error)
	// RevokeRefreshFamily kills every link of a sign-in at once.
	RevokeRefreshFamily(ctx context.Context, familyID string) error
	// SweepExpired deletes rows past expiry; returns how many went.
	SweepExpired(ctx context.Context) (int64, error)
}
