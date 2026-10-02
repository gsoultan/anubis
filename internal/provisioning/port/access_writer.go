package provisioningport

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/grant"
	"github.com/gsoultan/anubis/internal/authz/domain/membership"
)

// AccessWriter grants roles and adds people to memberships. As with
// PeopleWriter these are the authz context's admin usecases, so each
// write carries its own permission check and audit event.
type AccessWriter interface {
	CreateGrant(ctx context.Context, in grant.GrantCreate) (string, error)
	// AssignMembership is idempotent: an empty AssignmentID in the outcome
	// means the person already held the membership at that place.
	AssignMembership(ctx context.Context, in membership.MembershipAssignmentInput) (membership.MembershipAssignOutcome, error)
}
