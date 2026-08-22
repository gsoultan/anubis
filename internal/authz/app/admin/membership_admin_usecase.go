package authzadmin

import (
	"context"

	"github.com/gsoultan/anubis/internal/authz/domain/membership"
)

type MembershipAdminUsecase interface {
	ListMemberships(ctx context.Context) ([]membership.MembershipRecord, []membership.MembershipEntryRecord, []membership.MembershipEntryScopeRecord, error)
	CreateMembership(ctx context.Context, name, description string) (*membership.MembershipRecord, error)
	SetMembershipEntries(ctx context.Context, membershipID string, entries []membership.MembershipEntryInput) (int, error)
	AssignMembership(ctx context.Context, membershipID, identityID string) (int, error)
	UnassignMembership(ctx context.Context, membershipID, identityID string) (int, error)
	ResyncMembership(ctx context.Context, membershipID string) (int, error)
}
