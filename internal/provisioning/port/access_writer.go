package provisioningport

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/grant"
)

// AccessWriter grants roles and adds people to memberships. As with
// PeopleWriter these are the authz context's admin usecases, so each
// write carries its own permission check and audit event.
type AccessWriter interface {
	CreateGrant(ctx context.Context, in grant.GrantCreate) (string, error)
	AssignMembership(ctx context.Context, membershipID, identityID string) (int, error)
}
