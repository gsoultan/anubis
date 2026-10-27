package authport

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/share"
)

// LinkedShareRepository is the auth context's narrow view of record shares:
// enough to open a link, nothing to make or end one. The authz repository
// satisfies it structurally.
type LinkedShareRepository interface {
	ShareByLinkHash(ctx context.Context, hash []byte) (*share.Share, error)
	TouchShareLink(ctx context.Context, id string)
}
