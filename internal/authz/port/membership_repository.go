package authzport

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/membership"
)

type MembershipRepository interface {
	ListMemberships(ctx context.Context, tenantID string) ([]membership.MembershipRecord, error)
	MembershipByID(ctx context.Context, tenantID, id string) (*membership.MembershipRecord, error)
	CreateMembership(ctx context.Context, tenantID, name, description string) (string, error)
	MembershipEntries(ctx context.Context, membershipIDs []string) ([]membership.MembershipEntryRecord, error)
	MembershipEntryScopes(ctx context.Context, entryIDs []string) ([]membership.MembershipEntryScopeRecord, error)
	ReplaceMembershipEntries(ctx context.Context, tenantID, membershipID string, entries []membership.MembershipEntryInput) error
	AssignMembership(ctx context.Context, identityID, membershipID, assignedBy string) (int, error)
	UnassignMembership(ctx context.Context, identityID, membershipID string) (int, error)
	ResyncMembership(ctx context.Context, membershipID string) (int, error)
}
