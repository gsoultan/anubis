package usecase

import (
	"context"

	"github.com/gsoultan/anubis/internal/repository"
)

type MembershipAdminUsecase interface {
	ListMemberships(ctx context.Context) ([]repository.MembershipRecord, []repository.MembershipEntryRecord, []repository.MembershipEntryScopeRecord, error)
	CreateMembership(ctx context.Context, name, description string) (*repository.MembershipRecord, error)
	SetMembershipEntries(ctx context.Context, membershipID string, entries []repository.MembershipEntryInput) (int, error)
	AssignMembership(ctx context.Context, membershipID, identityID string) (int, error)
	UnassignMembership(ctx context.Context, membershipID, identityID string) (int, error)
	ResyncMembership(ctx context.Context, membershipID string) (int, error)
}
