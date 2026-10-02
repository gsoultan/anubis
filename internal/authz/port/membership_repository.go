package authzport

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/membership"
)

type MembershipRepository interface {
	ListMemberships(ctx context.Context, tenantID string) ([]membership.MembershipRecord, error)
	MembershipByID(ctx context.Context, tenantID, id string) (*membership.MembershipRecord, error)
	CreateMembership(ctx context.Context, tenantID, name, description, anchorAxis string) (string, error)
	MembershipEntries(ctx context.Context, membershipIDs []string) ([]membership.MembershipEntryRecord, error)
	MembershipEntryScopes(ctx context.Context, entryIDs []string) ([]membership.MembershipEntryScopeRecord, error)
	// ReplaceMembershipEntries makes the live entries equal the given set:
	// unchanged entries keep their id and their grants, gone ones are
	// retired, new ones are added. Nothing is deleted.
	ReplaceMembershipEntries(ctx context.Context, tenantID, membershipID string, entries []membership.MembershipEntryInput) error
	AssignMembership(ctx context.Context, in membership.MembershipAssignmentInput, assignedBy string) (membership.MembershipAssignOutcome, error)
	UnassignMembership(ctx context.Context, identityID, membershipID, removedBy, reason string) (int, error)
	LeaveMembership(ctx context.Context, tenantID, assignmentID, removedBy, reason string) (int, error)
	ResyncMembership(ctx context.Context, membershipID string) (int, error)
	ListMembershipAssignments(ctx context.Context, tenantID string, f membership.MembershipAssignmentFilter) ([]membership.MembershipAssignmentRecord, error)
}
