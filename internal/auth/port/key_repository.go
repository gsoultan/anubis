package authport

import (
	"context"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
)

// KeyRepository persists signing keys; the in-memory keyring is loaded from
// here and never queried per request.
type KeyRepository interface {
	VerificationKeys(ctx context.Context) ([]authdomain.KeyRecord, error)
	CreateKey(ctx context.Context, k authdomain.KeyRecord) error
	PromotePending(ctx context.Context, purpose string) (int64, error)
	DemoteActive(ctx context.Context, purpose string) (int64, error)
	SetKeyStatus(ctx context.Context, kid, status string) error
	SigningKeys(ctx context.Context) ([]authdomain.KeyRecord, error)
}
