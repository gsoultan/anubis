package authzadmin

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/membership"
)

type MembershipAdminUsecase interface {
	ListMemberships(ctx context.Context) ([]membership.MembershipRecord, []membership.MembershipEntryRecord, []membership.MembershipEntryScopeRecord, error)
	// CreateMembership fixes, once, where the membership applies: anchorAxis
	// empty gives every member the same places; set, each assignment names a
	// place on that axis.
	CreateMembership(ctx context.Context, name, description, anchorAxis string) (*membership.MembershipRecord, error)
	SetMembershipEntries(ctx context.Context, membershipID string, entries []membership.MembershipEntryInput) (int, error)
	AssignMembership(ctx context.Context, in membership.MembershipAssignmentInput) (membership.MembershipAssignOutcome, error)
	// UnassignMembership takes a person out of a membership at every place.
	UnassignMembership(ctx context.Context, membershipID, identityID, reason string) (int, error)
	// RemoveMembershipAssignment ends one assignment; the same person's other
	// places in the same membership stay.
	RemoveMembershipAssignment(ctx context.Context, assignmentID, reason string) (int, error)
	ResyncMembership(ctx context.Context, membershipID string) (int, error)
	// ListMembershipAssignments returns a page of current assignments and the
	// cursor for the next ("" when this was the last).
	ListMembershipAssignments(ctx context.Context, f membership.MembershipAssignmentFilter) ([]membership.MembershipAssignmentRecord, string, error)
}
