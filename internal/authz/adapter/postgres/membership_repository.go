package authzpg

import (
	"context"

	authzrquery "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rquery"
	"github.com/gsoultan/anubis/internal/authz/domain/membership"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) ListMemberships(ctx context.Context, tenantID string) ([]membership.MembershipRecord, error) {
	rows, err := authzrquery.ListMemberships.Query(ctx, s.rex(ctx), tenantID)
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
	r, ok, err := authzrquery.GetMembership.One(ctx, s.rex(ctx), id, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, apperr.ErrNotFound
	}
	return &membership.MembershipRecord{ID: r.ID, Name: r.Name, Description: r.Description}, nil
}

func (s *Repository) CreateMembership(ctx context.Context, tenantID, name, description string) (string, error) {
	row, ok, err := authzrquery.CreateMembership.One(ctx, s.rex(ctx), tenantID, name, description)
	if err != nil {
		return "", database.MapErr(err)
	}
	if !ok {
		return "", apperr.ErrNotFound
	}
	return row.ID, nil
}

func (s *Repository) MembershipEntries(ctx context.Context, membershipIDs []string) ([]membership.MembershipEntryRecord, error) {
	if len(membershipIDs) == 0 {
		return nil, nil
	}
	rows, err := authzrquery.ListMembershipEntries.Query(ctx, s.rex(ctx), membershipIDs)
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
	rows, err := authzrquery.ListMembershipEntryScopes.Query(ctx, s.rex(ctx), entryIDs)
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
		if _, err := authzrquery.DeleteMembershipEntries.Exec(ctx, s.rex(ctx), membershipID); err != nil {
			return database.MapErr(err)
		}
		for _, e := range entries {
			row, ok, err := authzrquery.InsertMembershipEntry.One(ctx, s.rex(ctx),
				membershipID, tenantID, e.RoleID)
			if err != nil {
				return database.MapErr(err)
			}
			if !ok {
				return apperr.ErrNotFound
			}
			for _, sc := range e.Scopes {
				if _, err := authzrquery.InsertMembershipEntryScope.Exec(ctx, s.rex(ctx),
					row.ID, tenantID, sc.Axis, sc.NodeID, sc.Inherit); err != nil {
					return database.MapErr(err)
				}
			}
		}
		return nil
	})
}

func (s *Repository) AssignMembership(ctx context.Context, identityID, membershipID, assignedBy string) (int, error) {
	row, ok, err := authzrquery.AssignMembership.One(ctx, s.rex(ctx),
		identityID, membershipID, assignedBy)
	if err != nil {
		return 0, database.MapErr(err)
	}
	if !ok {
		return 0, apperr.ErrNotFound
	}
	return int(row.GrantsCreated), nil
}

func (s *Repository) UnassignMembership(ctx context.Context, identityID, membershipID string) (int, error) {
	row, ok, err := authzrquery.UnassignMembership.One(ctx, s.rex(ctx), identityID, membershipID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	if !ok {
		return 0, apperr.ErrNotFound
	}
	return int(row.GrantsRevoked), nil
}

func (s *Repository) ResyncMembership(ctx context.Context, membershipID string) (int, error) {
	row, ok, err := authzrquery.ResyncMembership.One(ctx, s.rex(ctx), membershipID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	if !ok {
		return 0, apperr.ErrNotFound
	}
	return int(row.GrantsChanged), nil
}
