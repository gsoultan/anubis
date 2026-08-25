package controlport

import (
	"context"
	"time"

	controldomain "github.com/gsoultan/anubis/internal/control/domain"
)

// PlatformAPIKeyStore holds operators' machine credentials (0033). Only
// hashes cross this port; the secret exists once, in the response to the
// call that created it.
type PlatformAPIKeyStore interface {
	CreatePlatformAPIKey(ctx context.Context, ownerID, label, lookup, secretHash, createdBy string, expiresAt time.Time) (string, error)
	// PlatformAPIKeyByLookup returns nil when nothing matches — expired,
	// revoked and unknown are deliberately indistinguishable from outside.
	PlatformAPIKeyByLookup(ctx context.Context, lookup string) (*controldomain.PlatformAPIKeyAuth, error)
	TouchPlatformAPIKeyUsed(ctx context.Context, id string)
	ListPlatformAPIKeys(ctx context.Context) ([]controldomain.PlatformAPIKey, error)
	RevokePlatformAPIKey(ctx context.Context, id string) error
}
