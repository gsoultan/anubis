package authport

import (
	"context"

	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
)

// APIKeyRepository holds the tenant's machine credentials (migration 0030).
type APIKeyRepository interface {
	// CreateAPIKey stores the hash; the full key exists only in the response
	// that returns it once.
	CreateAPIKey(ctx context.Context, tenantID, label, lookup, secretHash, createdBy string, expiresAt *int64) (string, error)
	APIKeyByLookup(ctx context.Context, lookup string) (*authdomain.APIKeyAuth, error)
	ListAPIKeys(ctx context.Context, tenantID string) ([]authdomain.APIKeyRecord, error)
	RevokeAPIKey(ctx context.Context, tenantID, id string) error
	// TouchAPIKeyUsed is best effort: failing to note the time is not a
	// reason to refuse the caller.
	TouchAPIKeyUsed(ctx context.Context, id string)
}
