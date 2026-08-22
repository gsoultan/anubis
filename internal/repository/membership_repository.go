package repository

import "context"

type MembershipRepository interface {
	ListMemberships(ctx context.Context, tenantID string) ([]MembershipRecord, error)
	MembershipByID(ctx context.Context, tenantID, id string) (*MembershipRecord, error)
	CreateMembership(ctx context.Context, tenantID, name, description string) (string, error)
	MembershipEntries(ctx context.Context, membershipIDs []string) ([]MembershipEntryRecord, error)
	MembershipEntryScopes(ctx context.Context, entryIDs []string) ([]MembershipEntryScopeRecord, error)
	ReplaceMembershipEntries(ctx context.Context, tenantID, membershipID string, entries []MembershipEntryInput) error
	AssignMembership(ctx context.Context, identityID, membershipID, assignedBy string) (int, error)
	UnassignMembership(ctx context.Context, identityID, membershipID string) (int, error)
	ResyncMembership(ctx context.Context, membershipID string) (int, error)
}
