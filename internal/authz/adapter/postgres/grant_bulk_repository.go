package authzpg

import (
	"context"
	"encoding/json"

	authzrquery "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rquery"
	"github.com/gsoultan/anubis/internal/authz/domain/grantchange"
	"github.com/gsoultan/anubis/internal/authz/domain/insight"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// Bulk changes and access insight. The writes are set-based: a change to
// ten thousand grants is four statements, not forty thousand round trips.

func (s *Repository) ChangeCandidates(ctx context.Context, tenantID, roleID string, limit int) ([]grantchange.Candidate, error) {
	rows, err := authzrquery.ChangeCandidates.Query(ctx, s.rex(ctx), tenantID, roleID, int32(limit))
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]grantchange.Candidate, 0, len(rows))
	ids := make([]string, 0, len(rows))
	at := make(map[string]int, len(rows))
	for _, r := range rows {
		at[r.ID] = len(out)
		ids = append(ids, r.ID)
		out = append(out, grantchange.Candidate{
			GrantID: r.ID, IdentityID: r.IdentityID, Username: r.Username,
			SelfScoped: r.SelfScoped, FromGroup: r.FromGroup,
			State: grantchange.State{Until: r.ValidUntil.Ptr()},
		})
	}
	scopes, err := s.GrantScopes(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, sc := range scopes {
		c := &out[at[sc.GrantID]]
		c.State.Pins = append(c.State.Pins, grantchange.Pin{
			Axis: sc.Axis, NodeID: sc.NodeID, Inherit: sc.Inherit, Exclude: sc.Exclude,
		})
	}
	return out, nil
}

func (s *Repository) AxisFacts(ctx context.Context, tenantID, axis string) (bool, string, error) {
	row, ok, err := authzrquery.AxisFacts.One(ctx, s.rex(ctx), tenantID, axis)
	if err != nil {
		return false, "", database.MapErr(err)
	}
	if !ok {
		return false, "", apperr.ErrNotFound
	}
	return row.Deny, row.RootID, nil
}

func (s *Repository) NodeFamily(ctx context.Context, tenantID, nodeID string) (string, []string, []string, error) {
	row, ok, err := authzrquery.NodeFamily.One(ctx, s.rex(ctx), tenantID, nodeID)
	if err != nil {
		return "", nil, nil, database.MapErr(err)
	}
	if !ok {
		return "", nil, nil, apperr.ErrNotFound
	}
	return row.AxisCode, row.Siblings, row.Below, nil
}

// scopeRow and lifeRow are the JSON the set-based writes read.
type scopeRow struct {
	GrantID string `json:"grant_id"`
	grantchange.Pin
}

type lifeRow struct {
	GrantID string `json:"grant_id"`
	Until   *int64 `json:"until"`
	Revoked bool   `json:"revoked"`
}

// writeStates moves each grant from one state to another. Picks are only
// rewritten where they differ, so an end-date change does not churn
// grant_scopes (and the catalog version) for nothing.
func (s *Repository) writeStates(ctx context.Context, tenantID, reason string, items []grantchange.Item, to func(grantchange.Item) (grantchange.State, grantchange.State)) error {
	var rewrite []string
	var pins []scopeRow
	lives := make([]lifeRow, 0, len(items))
	for _, it := range items {
		from, next := to(it)
		if !from.SamePins(next) {
			rewrite = append(rewrite, it.GrantID)
			for _, p := range next.Pins {
				pins = append(pins, scopeRow{GrantID: it.GrantID, Pin: p})
			}
		}
		l := lifeRow{GrantID: it.GrantID, Revoked: next.Revoked}
		if next.Until != nil {
			u := next.Until.Unix()
			l.Until = &u
		}
		lives = append(lives, l)
	}
	if len(rewrite) > 0 {
		if _, err := authzrquery.DeleteScopesOfGrants.Exec(ctx, s.rex(ctx), tenantID, rewrite); err != nil {
			return database.MapErr(err)
		}
	}
	if len(pins) > 0 {
		b, err := json.Marshal(pins)
		if err != nil {
			return err
		}
		if _, err := authzrquery.InsertScopesFromJSON.Exec(ctx, s.rex(ctx), tenantID, string(b)); err != nil {
			return database.MapErr(err)
		}
	}
	b, err := json.Marshal(lives)
	if err != nil {
		return err
	}
	if _, err := authzrquery.UpdateGrantLives.Exec(ctx, s.rex(ctx), tenantID, string(b), reason); err != nil {
		return database.MapErr(err)
	}
	return nil
}

func (s *Repository) RecordGrantChange(ctx context.Context, tenantID, actorID string, spec grantchange.Spec, items []grantchange.Item) (string, error) {
	summary, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	var id string
	err = s.WithinTx(ctx, func(ctx context.Context) error {
		row, ok, err := authzrquery.InsertGrantChange.One(ctx, s.rex(ctx),
			tenantID, actorID, spec.Kind, spec.Reason, string(summary), int32(len(items)))
		if err != nil {
			return database.MapErr(err)
		}
		if !ok {
			return apperr.ErrNotFound
		}
		id = row.ID
		if err := s.writeStates(ctx, tenantID, spec.Reason, items, func(it grantchange.Item) (grantchange.State, grantchange.State) {
			return it.Before, it.After
		}); err != nil {
			return err
		}
		b, err := json.Marshal(items)
		if err != nil {
			return err
		}
		if _, err := authzrquery.InsertChangeItems.Exec(ctx, s.rex(ctx), id, tenantID, string(b)); err != nil {
			return database.MapErr(err)
		}
		return nil
	})
	return id, err
}

func changeRecord(r authzrquery.GrantChangeRow) grantchange.Record {
	return grantchange.Record{
		ID: r.ID, Kind: r.Kind, Reason: r.Reason, Grants: int(r.Grants),
		ActorID: r.ActorID, CreatedAt: r.CreatedAt, UndoneAt: r.UndoneAt.Ptr(), Summary: r.Summary,
	}
}

func (s *Repository) GrantChange(ctx context.Context, tenantID, id string) (*grantchange.Record, []grantchange.Item, error) {
	row, ok, err := authzrquery.GetGrantChangeForUpdate.One(ctx, s.rex(ctx), id, tenantID)
	if err != nil {
		return nil, nil, database.MapErr(err)
	}
	if !ok {
		return nil, nil, apperr.ErrNotFound
	}
	rows, err := authzrquery.ChangeItems.Query(ctx, s.rex(ctx), id, tenantID)
	if err != nil {
		return nil, nil, database.MapErr(err)
	}
	items := make([]grantchange.Item, 0, len(rows))
	for _, r := range rows {
		it := grantchange.Item{GrantID: r.GrantID}
		if err := json.Unmarshal([]byte(r.Before), &it.Before); err != nil {
			return nil, nil, err
		}
		if err := json.Unmarshal([]byte(r.After), &it.After); err != nil {
			return nil, nil, err
		}
		items = append(items, it)
	}
	rec := changeRecord(row)
	return &rec, items, nil
}

func (s *Repository) CurrentStates(ctx context.Context, tenantID string, grantIDs []string) (map[string]grantchange.State, error) {
	out := make(map[string]grantchange.State, len(grantIDs))
	if len(grantIDs) == 0 {
		return out, nil
	}
	rows, err := authzrquery.GrantStates.Query(ctx, s.rex(ctx), tenantID, grantIDs)
	if err != nil {
		return nil, database.MapErr(err)
	}
	for _, r := range rows {
		out[r.ID] = grantchange.State{Until: r.ValidUntil.Ptr(), Revoked: r.Revoked}
	}
	scopes, err := s.GrantScopes(ctx, grantIDs)
	if err != nil {
		return nil, err
	}
	for _, sc := range scopes {
		st := out[sc.GrantID]
		st.Pins = append(st.Pins, grantchange.Pin{Axis: sc.Axis, NodeID: sc.NodeID, Inherit: sc.Inherit, Exclude: sc.Exclude})
		out[sc.GrantID] = st
	}
	return out, nil
}

func (s *Repository) RestoreGrantChange(ctx context.Context, tenantID, id, actorID string, items []grantchange.Item) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.writeStates(ctx, tenantID, "", items, func(it grantchange.Item) (grantchange.State, grantchange.State) {
			return it.After, it.Before
		}); err != nil {
			return err
		}
		if _, err := authzrquery.MarkGrantChangeUndone.Exec(ctx, s.rex(ctx), id, tenantID, actorID); err != nil {
			return database.MapErr(err)
		}
		return nil
	})
}

func (s *Repository) ListGrantChanges(ctx context.Context, tenantID string, limit int) ([]grantchange.Record, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := authzrquery.ListGrantChanges.Query(ctx, s.rex(ctx), tenantID, int32(limit))
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]grantchange.Record, 0, len(rows))
	for _, r := range rows {
		out = append(out, changeRecord(r))
	}
	return out, nil
}

func (s *Repository) SummarizeAccess(ctx context.Context, tenantID string, identityIDs []string) ([]insight.AccessSummary, error) {
	if len(identityIDs) == 0 {
		return nil, nil
	}
	rows, err := authzrquery.SummarizeAccess.Query(ctx, s.rex(ctx), tenantID, identityIDs)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]insight.AccessSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, insight.AccessSummary{
			IdentityID: r.IdentityID, Direct: int(r.Direct), Groups: int(r.Groups),
			Roles: r.Roles, GroupNames: r.GroupNames, NextEnd: r.NextEnd.Ptr(),
		})
	}
	return out, nil
}

func (s *Repository) ListFullLists(ctx context.Context, tenantID, axis, roleID string) ([]insight.FullList, error) {
	rows, err := authzrquery.ListFullLists.Query(ctx, s.rex(ctx), tenantID, axis, roleID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]insight.FullList, 0, len(rows))
	for _, r := range rows {
		out = append(out, insight.FullList{
			Axis: r.AxisCode, ParentID: r.ParentID, ParentName: r.ParentName, IsRoot: r.IsRoot,
			RoleID: r.RoleID, RoleName: r.RoleName, Grants: int(r.Grants), Places: int(r.Places),
		})
	}
	return out, nil
}

func (s *Repository) PlaceReach(ctx context.Context, tenantID, nodeID string) (insight.PlaceReach, error) {
	var out insight.PlaceReach
	cover, ok, err := authzrquery.PlaceReachCover.One(ctx, s.rex(ctx), tenantID, nodeID)
	if err != nil {
		return out, database.MapErr(err)
	}
	if !ok {
		return out, apperr.ErrNotFound
	}
	out.HaveAll, out.HaveBranch, out.OtherFixed = int(cover.HaveAll), int(cover.HaveBranch), int(cover.OtherFixed)
	groups, err := authzrquery.PlaceReachMightExpect.Query(ctx, s.rex(ctx), tenantID, nodeID)
	if err != nil {
		return out, database.MapErr(err)
	}
	for _, g := range groups {
		out.MightExpect = append(out.MightExpect, insight.ReachGroup{
			RoleID: g.RoleID, RoleName: g.RoleName, People: int(g.People), Risk: g.Risk,
		})
	}
	entries, err := authzrquery.PlaceReachGroupEntries.Query(ctx, s.rex(ctx), tenantID, nodeID)
	if err != nil {
		return out, database.MapErr(err)
	}
	for _, e := range entries {
		out.GroupEntries = append(out.GroupEntries, insight.ReachEntry{
			MembershipID: e.MembershipID, MembershipName: e.MembershipName,
			RoleName: e.RoleName, Members: int(e.Members),
		})
	}
	return out, nil
}

func (s *Repository) RoleUsage(ctx context.Context, tenantID string, roleIDs []string) ([]insight.RoleUsage, error) {
	if roleIDs == nil {
		roleIDs = []string{}
	}
	rows, err := authzrquery.RoleUsage.Query(ctx, s.rex(ctx), tenantID, roleIDs)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]insight.RoleUsage, 0, len(rows))
	for _, r := range rows {
		out = append(out, insight.RoleUsage{RoleID: r.RoleID, People: int(r.People), ViaGroups: int(r.ViaGroups)})
	}
	return out, nil
}

func (s *Repository) AppUsage(ctx context.Context, tenantID, applicationID string) (insight.AppUsage, error) {
	r, _, err := authzrquery.AppUsage.One(ctx, s.rex(ctx), tenantID, applicationID)
	if err != nil {
		return insight.AppUsage{}, database.MapErr(err)
	}
	return insight.AppUsage{People: int(r.People), Direct: int(r.Direct), ViaGroups: int(r.ViaGroups)}, nil
}

func (s *Repository) HolderKinds(ctx context.Context, tenantID, roleID, axis string) (insight.HolderKinds, error) {
	r, _, err := authzrquery.HolderKinds.One(ctx, s.rex(ctx), tenantID, roleID, axis)
	if err != nil {
		return insight.HolderKinds{}, database.MapErr(err)
	}
	out := insight.HolderKinds{
		Total: int(r.Total), Everything: int(r.Everything), Branch: int(r.Branch),
		Fixed: int(r.Fixed), Nothing: int(r.Nothing),
	}
	full, err := s.ListFullLists(ctx, tenantID, axis, roleID)
	if err != nil {
		return out, err
	}
	for _, f := range full {
		out.Full += f.Grants
	}
	return out, nil
}

func (s *Repository) PlaceHolders(ctx context.Context, tenantID, nodeID string, limit int) ([]insight.PlaceHolder, error) {
	rows, err := authzrquery.PlaceHolders.Query(ctx, s.rex(ctx), tenantID, nodeID, int32(limit))
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]insight.PlaceHolder, 0, len(rows))
	for _, r := range rows {
		out = append(out, insight.PlaceHolder{
			IdentityID: r.IdentityID, Username: r.Username, RoleName: r.RoleName,
			GroupName: r.GroupName, OnlyHere: r.OnlyHere, FromAbove: r.FromAbove,
		})
	}
	return out, nil
}
