package authzpg

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/authz/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/authz/domain/membership"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) ListMemberships(ctx context.Context, tenantID string) ([]membership.MembershipRecord, error) {
	rows, err := s.q(ctx).ListMemberships(ctx, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]membership.MembershipRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, membership.MembershipRecord{
			ID: r.ID, Name: r.Name, Description: r.Description,
			MemberCount: int(r.MemberCount),
		})
	}
	return out, nil
}

func (s *Repository) MembershipByID(ctx context.Context, tenantID, id string) (*membership.MembershipRecord, error) {
	r, err := s.q(ctx).GetMembership(ctx, gen.GetMembershipParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &membership.MembershipRecord{ID: r.ID, Name: r.Name, Description: r.Description}, nil
}

func (s *Repository) CreateMembership(ctx context.Context, tenantID, name, description string) (string, error) {
	id, err := s.q(ctx).CreateMembership(ctx, gen.CreateMembershipParams{
		TenantID: tenantID, Name: name, Description: description,
	})
	return id, database.MapErr(err)
}

func (s *Repository) MembershipEntries(ctx context.Context, membershipIDs []string) ([]membership.MembershipEntryRecord, error) {
	if len(membershipIDs) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListMembershipEntries(ctx, membershipIDs)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]membership.MembershipEntryRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, membership.MembershipEntryRecord{
			ID: r.ID, MembershipID: r.MembershipID, RoleID: r.RoleID, RoleName: r.RoleName,
		})
	}
	return out, nil
}

func (s *Repository) MembershipEntryScopes(ctx context.Context, entryIDs []string) ([]membership.MembershipEntryScopeRecord, error) {
	if len(entryIDs) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListMembershipEntryScopes(ctx, entryIDs)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]membership.MembershipEntryScopeRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, membership.MembershipEntryScopeRecord{
			EntryID: r.EntryID, Axis: r.AxisCode, NodeID: r.ScopeNodeID,
			NodeName: r.NodeName, Inherit: r.Inherit,
		})
	}
	return out, nil
}

func (s *Repository) ReplaceMembershipEntries(ctx context.Context, tenantID, membershipID string, entries []membership.MembershipEntryInput) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.q(ctx).DeleteMembershipEntries(ctx, membershipID); err != nil {
			return database.MapErr(err)
		}
		for _, e := range entries {
			entryID, err := s.q(ctx).InsertMembershipEntry(ctx, gen.InsertMembershipEntryParams{
				MembershipID: membershipID, TenantID: tenantID, RoleID: e.RoleID,
			})
			if err != nil {
				return database.MapErr(err)
			}
			for _, sc := range e.Scopes {
				if err := s.q(ctx).InsertMembershipEntryScope(ctx, gen.InsertMembershipEntryScopeParams{
					EntryID: entryID, TenantID: tenantID, AxisCode: sc.Axis,
					ScopeNodeID: sc.NodeID, Inherit: sc.Inherit,
				}); err != nil {
					return database.MapErr(err)
				}
			}
		}
		return nil
	})
}

func (s *Repository) AssignMembership(ctx context.Context, identityID, membershipID, assignedBy string) (int, error) {
	n, err := s.q(ctx).AssignMembership(ctx, gen.AssignMembershipParams{
		IdentityID: identityID, MembershipID: membershipID, AssignedBy: assignedBy,
	})
	return int(n), database.MapErr(err)
}

func (s *Repository) UnassignMembership(ctx context.Context, identityID, membershipID string) (int, error) {
	n, err := s.q(ctx).UnassignMembership(ctx, gen.UnassignMembershipParams{
		IdentityID: identityID, MembershipID: membershipID,
	})
	return int(n), database.MapErr(err)
}

func (s *Repository) ResyncMembership(ctx context.Context, membershipID string) (int, error) {
	n, err := s.q(ctx).ResyncMembership(ctx, membershipID)
	return int(n), database.MapErr(err)
}
