package authzpg

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
			AnchorAxis: r.AnchorAxis.V, MemberCount: int(r.MemberCount),
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
	return &membership.MembershipRecord{
		ID: r.ID, Name: r.Name, Description: r.Description, AnchorAxis: r.AnchorAxis.V,
	}, nil
}

func (s *Repository) CreateMembership(ctx context.Context, tenantID, name, description, anchorAxis string) (string, error) {
	row, ok, err := authzrquery.CreateMembership.One(ctx, s.rex(ctx), tenantID, name, description, anchorAxis)
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
			NodeName: r.NodeName, Inherit: r.Inherit, Exclude: r.Exclude,
		})
	}
	return out, nil
}

// entryKey is what makes two entries the same entry: the role and the exact
// set of places. Order-free, so re-sending a membership's contents in a
// different order changes nothing.
func entryKey(roleID string, places []string) string {
	sort.Strings(places)
	return strings.ToLower(roleID) + "|" + strings.Join(places, ",")
}

func placeKey(axis, nodeID string, inherit, exclude bool) string {
	return fmt.Sprintf("%s:%s:%t:%t", axis, strings.ToLower(nodeID), inherit, exclude)
}

// ReplaceMembershipEntries makes the membership's live entries equal the
// given set, and deletes nothing (0054).
//
// It used to DELETE every entry and insert the new list. Grants fanned out
// from an entry point back at it with ON DELETE RESTRICT, and grants are kept
// after they are revoked, so the first person ever assigned froze the
// membership: every later change was refused by the foreign key. Now an entry
// that did not change keeps its id — and its members keep their grants
// untouched — an entry that went is retired, and one that came is added. The
// caller's resync then revokes what the retired entries gave and hands out
// what the new ones give.
func (s *Repository) ReplaceMembershipEntries(ctx context.Context, tenantID, membershipID string, entries []membership.MembershipEntryInput) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		live, err := s.MembershipEntries(ctx, []string{membershipID})
		if err != nil {
			return err
		}
		ids := make([]string, len(live))
		for i, e := range live {
			ids[i] = e.ID
		}
		scopes, err := s.MembershipEntryScopes(ctx, ids)
		if err != nil {
			return err
		}
		placesOf := map[string][]string{}
		for _, sc := range scopes {
			placesOf[sc.EntryID] = append(placesOf[sc.EntryID],
				placeKey(sc.Axis, sc.NodeID, sc.Inherit, sc.Exclude))
		}
		// A multiset: the same role at the same places may legitimately appear
		// twice, and each occurrence keeps at most one existing entry.
		have := map[string][]string{}
		for _, e := range live {
			k := entryKey(e.RoleID, placesOf[e.ID])
			have[k] = append(have[k], e.ID)
		}
		var add []membership.MembershipEntryInput
		for _, in := range entries {
			places := make([]string, 0, len(in.Scopes))
			for _, sc := range in.Scopes {
				places = append(places, placeKey(sc.Axis, sc.NodeID, sc.Inherit, sc.Exclude))
			}
			k := entryKey(in.RoleID, places)
			if kept := have[k]; len(kept) > 0 {
				have[k] = kept[1:]
				continue
			}
			add = append(add, in)
		}
		var retire []string
		for _, gone := range have {
			retire = append(retire, gone...)
		}
		if len(retire) > 0 {
			if _, err := authzrquery.RetireMembershipEntries.Exec(ctx, s.rex(ctx), membershipID, retire); err != nil {
				return database.MapErrSaying(err)
			}
		}
		for _, e := range add {
			row, ok, err := authzrquery.InsertMembershipEntry.One(ctx, s.rex(ctx),
				membershipID, tenantID, e.RoleID)
			if err != nil {
				return database.MapErrSaying(err)
			}
			if !ok {
				return apperr.ErrNotFound
			}
			for _, sc := range e.Scopes {
				if _, err := authzrquery.InsertMembershipEntryScope.Exec(ctx, s.rex(ctx),
					row.ID, tenantID, sc.Axis, sc.NodeID, sc.Inherit, sc.Exclude); err != nil {
					return database.MapErrSaying(err)
				}
			}
		}
		return nil
	})
}

func (s *Repository) AssignMembership(ctx context.Context, in membership.MembershipAssignmentInput, assignedBy string) (membership.MembershipAssignOutcome, error) {
	var out membership.MembershipAssignOutcome
	err := s.WithinTx(ctx, func(ctx context.Context) error {
		row, ok, err := authzrquery.AssignMembership.One(ctx, s.rex(ctx),
			in.IdentityID, in.MembershipID, assignedBy, in.NodeID, !in.Exact,
			database.OptTime(in.ValidUntil), in.Reason)
		if err != nil {
			return database.MapErrSaying(err)
		}
		if !ok {
			return apperr.ErrNotFound
		}
		out.AssignmentID = row.AssignmentID
		if out.AssignmentID == "" {
			return nil // already held there: nothing was created
		}
		n, ok, err := authzrquery.CountAssignmentGrants.One(ctx, s.rex(ctx), out.AssignmentID)
		if err != nil {
			return database.MapErrSaying(err)
		}
		if ok {
			out.GrantsCreated = int(n.Count)
		}
		return nil
	})
	return out, err
}

func (s *Repository) UnassignMembership(ctx context.Context, identityID, membershipID, removedBy, reason string) (int, error) {
	row, ok, err := authzrquery.UnassignMembership.One(ctx, s.rex(ctx), identityID, membershipID, removedBy, reason)
	if err != nil {
		return 0, database.MapErr(err)
	}
	if !ok {
		return 0, apperr.ErrNotFound
	}
	return int(row.GrantsRevoked), nil
}

func (s *Repository) LeaveMembership(ctx context.Context, tenantID, assignmentID, removedBy, reason string) (int, error) {
	row, ok, err := authzrquery.LeaveMembership.One(ctx, s.rex(ctx), assignmentID, tenantID, removedBy, reason)
	if err != nil {
		return 0, database.MapErr(err)
	}
	if !ok {
		// No current assignment with that id in this tenant: somebody else's,
		// already ended, or never there. The same answer for all three.
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

func (s *Repository) ListMembershipAssignments(ctx context.Context, tenantID string, f membership.MembershipAssignmentFilter) ([]membership.MembershipAssignmentRecord, error) {
	size := f.PageSize
	if size <= 0 {
		size = 50
	}
	if size > 200 {
		size = 200
	}
	rows, err := authzrquery.ListMembershipAssignments.Query(ctx, s.rex(ctx),
		tenantID, f.MembershipID, f.IdentityID, f.Cursor, int32(size))
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]membership.MembershipAssignmentRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, membership.MembershipAssignmentRecord{
			ID: r.ID, MembershipID: r.MembershipID, MembershipName: r.MembershipName,
			AnchorAxis: r.AnchorAxis.V, IdentityID: r.IdentityID, Username: r.Username,
			NodeID: r.NodeID.V, NodeName: r.NodeName.V, Exact: !r.Inherit,
			ValidUntil: r.ValidUntil.Ptr(), Reason: r.Reason.V,
			AssignedAt: r.AssignedAt, AssignedBy: r.AssignedBy,
		})
	}
	return out, nil
}
