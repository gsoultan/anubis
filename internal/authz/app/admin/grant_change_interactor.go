package authzadmin

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gsoultan/anubis/internal/authz/domain/grantchange"
	"github.com/gsoultan/anubis/internal/authz/domain/insight"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// maxSummarized is one page of the People list.
const maxSummarized = 200

func (u *authzAdminInteractor) SummarizeAccess(ctx context.Context, identityIDs []string) ([]insight.AccessSummary, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	if len(identityIDs) > maxSummarized {
		return nil, apperr.ErrInvalidArgument.With("identity_ids", "at most 200 at a time")
	}
	return u.insight.SummarizeAccess(ctx, p.TenantID, identityIDs)
}

func (u *authzAdminInteractor) ListFullLists(ctx context.Context, axis, roleID string) ([]insight.FullList, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	return u.insight.ListFullLists(ctx, p.TenantID, strings.TrimSpace(axis), strings.TrimSpace(roleID))
}

func (u *authzAdminInteractor) PlaceReach(ctx context.Context, nodeID string) (insight.PlaceReach, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return insight.PlaceReach{}, err
	}
	if strings.TrimSpace(nodeID) == "" {
		return insight.PlaceReach{}, apperr.ErrInvalidArgument.With("node_id", "required")
	}
	return u.insight.PlaceReach(ctx, p.TenantID, nodeID)
}

// maxRoleUsage bounds one RoleUsage call; empty asks for every role.
const maxRoleUsage = 500

func (u *authzAdminInteractor) RoleUsage(ctx context.Context, roleIDs []string) ([]insight.RoleUsage, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	if len(roleIDs) > maxRoleUsage {
		return nil, apperr.ErrInvalidArgument.With("role_ids", "at most 500 at a time")
	}
	return u.insight.RoleUsage(ctx, p.TenantID, roleIDs)
}

func (u *authzAdminInteractor) AppUsage(ctx context.Context, applicationID string) (insight.AppUsage, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return insight.AppUsage{}, err
	}
	if strings.TrimSpace(applicationID) == "" {
		return insight.AppUsage{}, apperr.ErrInvalidArgument.With("application_id", "required")
	}
	return u.insight.AppUsage(ctx, p.TenantID, applicationID)
}

func (u *authzAdminInteractor) HolderKinds(ctx context.Context, roleID, axis string) (insight.HolderKinds, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return insight.HolderKinds{}, err
	}
	if strings.TrimSpace(roleID) == "" || strings.TrimSpace(axis) == "" {
		return insight.HolderKinds{}, apperr.ErrInvalidArgument.With("role_id", "a role and a kind of place")
	}
	return u.insight.HolderKinds(ctx, p.TenantID, roleID, axis)
}

// maxPlaceHolders is the most "Who has access here" lists by name.
const maxPlaceHolders = 100

func (u *authzAdminInteractor) PlaceHolders(ctx context.Context, nodeID string, limit int) ([]insight.PlaceHolder, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(nodeID) == "" {
		return nil, apperr.ErrInvalidArgument.With("node_id", "required")
	}
	if limit <= 0 || limit > maxPlaceHolders {
		limit = maxPlaceHolders
	}
	return u.insight.PlaceHolders(ctx, p.TenantID, nodeID, limit)
}

func (u *authzAdminInteractor) ListGrantChanges(ctx context.Context, limit int) ([]grantchange.Record, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	return u.changes.ListGrantChanges(ctx, p.TenantID, limit)
}

// checkSpec refuses a change that does not say enough to be read back.
func (u *authzAdminInteractor) checkSpec(s grantchange.Spec) error {
	bad := func(field, why string) error { return apperr.ErrInvalidArgument.With(field, why) }
	if s.RoleID == "" {
		return bad("role_id", "a change is always about one role")
	}
	switch s.Kind {
	case grantchange.AddPlace, grantchange.RemovePlace:
		if s.NodeID == "" {
			return bad("node_id", "which place")
		}
	case grantchange.SwapPlace:
		if s.NodeID == "" || s.ToNodeID == "" || s.NodeID == s.ToNodeID {
			return bad("to_node_id", "a swap needs two different places")
		}
	case grantchange.Grow:
	case grantchange.SetUntil:
		if s.Until != nil && !s.Until.After(u.now()) {
			return bad("valid_until", "must be in the future")
		}
	case grantchange.Revoke:
		if strings.TrimSpace(s.Reason) == "" {
			return bad("reason", "say why the access is being taken away")
		}
	default:
		return bad("kind", "unknown change")
	}
	if (s.NeedsAxis() || len(s.Covers) > 0 || s.SiblingsOf != "") && s.Axis == "" {
		return bad("axis", "which kind of place")
	}
	return nil
}

// plan works out a change from the grants as they stand. Run inside the
// transaction that applies it, the grants are locked while it is built.
func (u *authzAdminInteractor) plan(ctx context.Context, tenantID string, s grantchange.Spec) (grantchange.Plan, error) {
	if err := u.checkSpec(s); err != nil {
		return grantchange.Plan{}, err
	}
	var f grantchange.AxisFacts
	if s.Axis != "" {
		deny, root, err := u.changes.AxisFacts(ctx, tenantID, s.Axis)
		if err != nil {
			return grantchange.Plan{}, apperr.ErrInvalidArgument.With("axis", "no such kind of place")
		}
		f.Deny, f.RootID = deny, root
	}
	// Every place named has to be on the axis the change is about: a pick
	// on the wrong kind of place would be refused by the database anyway,
	// but only after the plan said it would work.
	for _, n := range []string{s.NodeID, s.ToNodeID, s.SiblingsOf} {
		if n == "" {
			continue
		}
		axis, siblings, below, err := u.changes.NodeFamily(ctx, tenantID, n)
		if err != nil || axis != s.Axis {
			return grantchange.Plan{}, apperr.ErrInvalidArgument.With("node_id", "not a place of this kind")
		}
		if n == s.SiblingsOf {
			f.Siblings = siblings
		}
		if n == s.NodeID && s.Kind == grantchange.Grow {
			f.Below = make(map[string]bool, len(below))
			for _, b := range below {
				f.Below[b] = true
			}
		}
	}
	cands, err := u.changes.ChangeCandidates(ctx, tenantID, s.RoleID, grantchange.MaxGrants+1)
	if err != nil {
		return grantchange.Plan{}, err
	}
	p, err := grantchange.Build(s, cands, f)
	if errors.Is(err, grantchange.ErrTooMany) {
		return p, apperr.ErrInvalidArgument.With("filter", err.Error())
	}
	return p, err
}

func (u *authzAdminInteractor) PreviewGrantChange(ctx context.Context, s grantchange.Spec) (grantchange.Plan, error) {
	p, err := u.guard.Require(ctx, "anubis:grant:admin")
	if err != nil {
		return grantchange.Plan{}, err
	}
	// A read, but in a transaction all the same: the candidate query locks,
	// and locking outside one is an error. Nothing is written.
	var out grantchange.Plan
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = u.plan(ctx, p.TenantID, s)
		return err
	})
	return out, err
}

func (u *authzAdminInteractor) ApplyGrantChange(ctx context.Context, s grantchange.Spec) (string, grantchange.Plan, error) {
	p, err := u.guard.Require(ctx, "anubis:grant:admin")
	if err != nil {
		return "", grantchange.Plan{}, err
	}
	var id string
	var plan grantchange.Plan
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		plan, err = u.plan(ctx, p.TenantID, s)
		if err != nil {
			return err
		}
		if len(plan.Items) == 0 {
			return apperr.ErrInvalidArgument.With("change", "nothing to change")
		}
		id, err = u.changes.RecordGrantChange(ctx, p.TenantID, p.IdentityID, s, plan.Items)
		return err
	})
	if err != nil {
		return "", plan, err
	}
	u.emit(ctx, p, "grant.change.apply", id, map[string]string{
		"kind": s.Kind, "role_id": s.RoleID, "grants": strconv.Itoa(len(plan.Items)), "reason": s.Reason,
	})
	return id, plan, nil
}

// UndoGrantChange puts every grant a change touched back as it was, but
// only if every one of them is still exactly as the change left it. Undoing
// over a later edit would silently throw that edit away; a person can undo
// the later change first, or edit by hand.
func (u *authzAdminInteractor) UndoGrantChange(ctx context.Context, changeID, reason string) (int, error) {
	p, err := u.guard.Require(ctx, "anubis:grant:admin")
	if err != nil {
		return 0, err
	}
	var n int
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		rec, items, err := u.changes.GrantChange(ctx, p.TenantID, changeID)
		if err != nil {
			return err
		}
		if rec.UndoneAt != nil {
			return apperr.ErrConflict.With("change", "already undone")
		}
		ids := make([]string, len(items))
		for i, it := range items {
			ids[i] = it.GrantID
		}
		now, err := u.changes.CurrentStates(ctx, p.TenantID, ids)
		if err != nil {
			return err
		}
		moved := 0
		for _, it := range items {
			if cur, ok := now[it.GrantID]; !ok || !cur.Same(it.After) {
				moved++
			}
		}
		if moved > 0 {
			return apperr.ErrConflict.With("change",
				strconv.Itoa(moved)+" of these grants changed since; undo those changes first")
		}
		n = len(items)
		return u.changes.RestoreGrantChange(ctx, p.TenantID, changeID, p.IdentityID, items)
	})
	if err != nil {
		return 0, err
	}
	u.emit(ctx, p, "grant.change.undo", changeID, map[string]string{"grants": strconv.Itoa(n), "reason": reason})
	return n, nil
}
