package authzshareport

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/share"
)

// ShareRepository stores record shares. Lists return live shares only: a
// revoked or lapsed share is not something anybody acts on.
type ShareRepository interface {
	CreateShare(ctx context.Context, in share.CreateInput) (*share.Share, error)
	Share(ctx context.Context, tenantID, id string) (*share.Share, error)
	SharesOnRecord(ctx context.Context, tenantID, record string) ([]share.Share, error)
	SharesForRecipient(ctx context.Context, tenantID, identityID string) ([]share.Share, error)
	// RevokeShare reports whether this call revoked it; false means it was
	// already revoked, which the caller treats as done.
	RevokeShare(ctx context.Context, tenantID, id, reason string) (bool, error)
	// SetShareLink installs or replaces the access link on a live share;
	// false means the share is not live.
	SetShareLink(ctx context.Context, tenantID, id string, hash []byte) (bool, error)
	// ShareByLinkHash is the live share behind a presented link, or not found.
	ShareByLinkHash(ctx context.Context, hash []byte) (*share.Share, error)
	TouchShareLink(ctx context.Context, id string)
}
