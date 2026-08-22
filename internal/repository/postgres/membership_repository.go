package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) ListMemberships(ctx context.Context, tenantID string) ([]repository.MembershipRecord, error) {
	rows, err := s.q(ctx).ListMemberships(ctx, tenantID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.MembershipRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.MembershipRecord{
			ID: r.ID, Name: r.Name, Description: r.Description,
			MemberCount: int(r.MemberCount),
		})
	}
	return out, nil
}

func (s *Store) MembershipByID(ctx context.Context, tenantID, id string) (*repository.MembershipRecord, error) {
	r, err := s.q(ctx).GetMembership(ctx, gen.GetMembershipParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.MembershipRecord{ID: r.ID, Name: r.Name, Description: r.Description}, nil
}

func (s *Store) CreateMembership(ctx context.Context, tenantID, name, description string) (string, error) {
	id, err := s.q(ctx).CreateMembership(ctx, gen.CreateMembershipParams{
		TenantID: tenantID, Name: name, Description: description,
	})
	return id, mapErr(err)
}

func (s *Store) MembershipEntries(ctx context.Context, membershipIDs []string) ([]repository.MembershipEntryRecord, error) {
	if len(membershipIDs) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListMembershipEntries(ctx, membershipIDs)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.MembershipEntryRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.MembershipEntryRecord{
			ID: r.ID, MembershipID: r.MembershipID, RoleID: r.RoleID, RoleName: r.RoleName,
		})
	}
	return out, nil
}

func (s *Store) MembershipEntryScopes(ctx context.Context, entryIDs []string) ([]repository.MembershipEntryScopeRecord, error) {
	if len(entryIDs) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListMembershipEntryScopes(ctx, entryIDs)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.MembershipEntryScopeRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.MembershipEntryScopeRecord{
			EntryID: r.EntryID, Axis: r.AxisCode, NodeID: r.ScopeNodeID,
			NodeName: r.NodeName, Inherit: r.Inherit,
		})
	}
	return out, nil
}

func (s *Store) ReplaceMembershipEntries(ctx context.Context, tenantID, membershipID string, entries []repository.MembershipEntryInput) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.q(ctx).DeleteMembershipEntries(ctx, membershipID); err != nil {
			return mapErr(err)
		}
		for _, e := range entries {
			entryID, err := s.q(ctx).InsertMembershipEntry(ctx, gen.InsertMembershipEntryParams{
				MembershipID: membershipID, TenantID: tenantID, RoleID: e.RoleID,
			})
			if err != nil {
				return mapErr(err)
			}
			for _, sc := range e.Scopes {
				if err := s.q(ctx).InsertMembershipEntryScope(ctx, gen.InsertMembershipEntryScopeParams{
					EntryID: entryID, TenantID: tenantID, AxisCode: sc.Axis,
					ScopeNodeID: sc.NodeID, Inherit: sc.Inherit,
				}); err != nil {
					return mapErr(err)
				}
			}
		}
		return nil
	})
}

func (s *Store) AssignMembership(ctx context.Context, identityID, membershipID, assignedBy string) (int, error) {
	n, err := s.q(ctx).AssignMembership(ctx, gen.AssignMembershipParams{
		IdentityID: identityID, MembershipID: membershipID, AssignedBy: assignedBy,
	})
	return int(n), mapErr(err)
}

func (s *Store) UnassignMembership(ctx context.Context, identityID, membershipID string) (int, error) {
	n, err := s.q(ctx).UnassignMembership(ctx, gen.UnassignMembershipParams{
		IdentityID: identityID, MembershipID: membershipID,
	})
	return int(n), mapErr(err)
}

func (s *Store) ResyncMembership(ctx context.Context, membershipID string) (int, error) {
	n, err := s.q(ctx).ResyncMembership(ctx, membershipID)
	return int(n), mapErr(err)
}
