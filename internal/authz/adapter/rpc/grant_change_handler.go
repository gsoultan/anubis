package authzrpc

import (
	"context"
	"sort"
	"time"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	"github.com/gsoultan/anubis/internal/authz/domain/grantchange"
)

// The access-as-a-whole half of AuthzAdminService: summaries for the People
// list, bulk changes, and the place-reach questions.

func (h *AuthzAdminHandler) SummarizeAccess(ctx context.Context, req *connect.Request[anubisv1.SummarizeAccessRequest]) (*connect.Response[anubisv1.SummarizeAccessResponse], error) {
	out, err := h.f.Do(ctx, "admin.access.summarize", func(ctx context.Context) (any, error) {
		rows, err := h.svc.SummarizeAccess(ctx, req.Msg.IdentityIds)
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.SummarizeAccessResponse{}
		for _, r := range rows {
			s := &anubisv1.AccessSummary{
				IdentityId: r.IdentityID, Direct: int32(r.Direct), Groups: int32(r.Groups),
				Roles: r.Roles, GroupNames: r.GroupNames,
			}
			if r.NextEnd != nil {
				s.NextEnd = r.NextEnd.Unix()
			}
			resp.Summaries = append(resp.Summaries, s)
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.SummarizeAccessResponse)), nil
}

func changeSpec(c *anubisv1.GrantChangeSpec) grantchange.Spec {
	if c == nil {
		return grantchange.Spec{}
	}
	s := grantchange.Spec{
		Kind: c.Kind, Axis: c.Axis, NodeID: c.NodeId, ToNodeID: c.ToNodeId,
		Inherit: c.Inherit, Reason: c.Reason,
	}
	if f := c.Filter; f != nil {
		s.RoleID, s.Covers, s.SiblingsOf = f.RoleId, f.CoversNodeIds, f.SiblingsOf
		// The filter's axis and the change's axis are one axis: a filter on
		// one kind of place and a change on another would match on one thing
		// and act on another, which no preview can explain.
		if s.Axis == "" {
			s.Axis = f.Axis
		}
	}
	if c.ValidUntil > 0 {
		t := time.Unix(c.ValidUntil, 0)
		s.Until = &t
	}
	return s
}

func pinProtos(ps []grantchange.Pin) []*anubisv1.GrantScope {
	out := make([]*anubisv1.GrantScope, 0, len(ps))
	for _, p := range ps {
		out = append(out, scopeProto(p.Axis, p.NodeID, "", p.Inherit, p.Exclude))
	}
	return out
}

func unixOr0(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.Unix()
}

func skipProtos(m map[string]int) []*anubisv1.GrantChangeSkip {
	out := make([]*anubisv1.GrantChangeSkip, 0, len(m))
	for reason, n := range m {
		out = append(out, &anubisv1.GrantChangeSkip{Reason: reason, Count: int32(n)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// previewSample is how many rows of a preview are sent: enough to read, not
// the whole change.
const previewSample = 20

func (h *AuthzAdminHandler) PreviewGrantChange(ctx context.Context, req *connect.Request[anubisv1.PreviewGrantChangeRequest]) (*connect.Response[anubisv1.PreviewGrantChangeResponse], error) {
	out, err := h.f.Do(ctx, "admin.grant.change.preview", func(ctx context.Context) (any, error) {
		p, err := h.svc.PreviewGrantChange(ctx, changeSpec(req.Msg.Change))
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.PreviewGrantChangeResponse{
			Matched: int32(p.Matched), Changes: int32(len(p.Items)), People: int32(p.People()),
			Skipped: skipProtos(p.Skipped),
		}
		for i, it := range p.Items {
			if i == previewSample {
				break
			}
			resp.Sample = append(resp.Sample, &anubisv1.GrantChangeRow{
				GrantId: it.GrantID, IdentityId: it.IdentityID, Username: it.Username,
				Before: pinProtos(it.Before.Pins), After: pinProtos(it.After.Pins),
				BeforeUntil: unixOr0(it.Before.Until), AfterUntil: unixOr0(it.After.Until),
				Revoked: it.After.Revoked,
			})
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.PreviewGrantChangeResponse)), nil
}

func (h *AuthzAdminHandler) ApplyGrantChange(ctx context.Context, req *connect.Request[anubisv1.ApplyGrantChangeRequest]) (*connect.Response[anubisv1.ApplyGrantChangeResponse], error) {
	out, err := h.f.Do(ctx, "admin.grant.change.apply", func(ctx context.Context) (any, error) {
		id, p, err := h.svc.ApplyGrantChange(ctx, changeSpec(req.Msg.Change))
		if err != nil {
			return nil, err
		}
		return &anubisv1.ApplyGrantChangeResponse{
			ChangeId: id, Changed: int32(len(p.Items)), Skipped: skipProtos(p.Skipped),
		}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ApplyGrantChangeResponse)), nil
}

func (h *AuthzAdminHandler) UndoGrantChange(ctx context.Context, req *connect.Request[anubisv1.UndoGrantChangeRequest]) (*connect.Response[anubisv1.UndoGrantChangeResponse], error) {
	out, err := h.f.Do(ctx, "admin.grant.change.undo", func(ctx context.Context) (any, error) {
		return h.svc.UndoGrantChange(ctx, req.Msg.ChangeId, req.Msg.Reason)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.UndoGrantChangeResponse{Restored: int32(out.(int))}), nil
}

func (h *AuthzAdminHandler) ListGrantChanges(ctx context.Context, req *connect.Request[anubisv1.ListGrantChangesRequest]) (*connect.Response[anubisv1.ListGrantChangesResponse], error) {
	out, err := h.f.Do(ctx, "admin.grant.change.list", func(ctx context.Context) (any, error) {
		rows, err := h.svc.ListGrantChanges(ctx, int(req.Msg.Limit))
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.ListGrantChangesResponse{}
		for _, r := range rows {
			resp.Changes = append(resp.Changes, &anubisv1.GrantChange{
				Id: r.ID, Kind: r.Kind, Reason: r.Reason, Grants: int32(r.Grants),
				CreatedAt: r.CreatedAt.Unix(), ActorId: r.ActorID, UndoneAt: unixOr0(r.UndoneAt),
				SummaryJson: r.Summary,
			})
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ListGrantChangesResponse)), nil
}

func (h *AuthzAdminHandler) ListFullLists(ctx context.Context, req *connect.Request[anubisv1.ListFullListsRequest]) (*connect.Response[anubisv1.ListFullListsResponse], error) {
	out, err := h.f.Do(ctx, "admin.access.full_lists", func(ctx context.Context) (any, error) {
		rows, err := h.svc.ListFullLists(ctx, req.Msg.Axis, req.Msg.RoleId)
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.ListFullListsResponse{}
		for _, r := range rows {
			resp.Lists = append(resp.Lists, &anubisv1.FullList{
				Axis: r.Axis, ParentId: r.ParentID, ParentName: r.ParentName, IsRoot: r.IsRoot,
				RoleId: r.RoleID, RoleName: r.RoleName, Grants: int32(r.Grants), Places: int32(r.Places),
			})
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ListFullListsResponse)), nil
}

func (h *AuthzAdminHandler) PlaceReach(ctx context.Context, req *connect.Request[anubisv1.PlaceReachRequest]) (*connect.Response[anubisv1.PlaceReachResponse], error) {
	out, err := h.f.Do(ctx, "admin.access.place_reach", func(ctx context.Context) (any, error) {
		r, err := h.svc.PlaceReach(ctx, req.Msg.NodeId)
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.PlaceReachResponse{
			HaveAll: int32(r.HaveAll), HaveBranch: int32(r.HaveBranch), OtherFixed: int32(r.OtherFixed),
		}
		for _, g := range r.MightExpect {
			resp.MightExpect = append(resp.MightExpect, &anubisv1.PlaceReachGroup{
				RoleId: g.RoleID, RoleName: g.RoleName, People: int32(g.People), Risk: g.Risk,
			})
		}
		for _, e := range r.GroupEntries {
			resp.GroupEntries = append(resp.GroupEntries, &anubisv1.PlaceReachEntry{
				MembershipId: e.MembershipID, MembershipName: e.MembershipName,
				RoleName: e.RoleName, Members: int32(e.Members),
			})
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.PlaceReachResponse)), nil
}

func (h *AuthzAdminHandler) RoleUsage(ctx context.Context, req *connect.Request[anubisv1.RoleUsageRequest]) (*connect.Response[anubisv1.RoleUsageResponse], error) {
	out, err := h.f.Do(ctx, "admin.access.role_usage", func(ctx context.Context) (any, error) {
		rows, err := h.svc.RoleUsage(ctx, req.Msg.RoleIds)
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.RoleUsageResponse{}
		for _, r := range rows {
			resp.Roles = append(resp.Roles, &anubisv1.RoleUsage{
				RoleId: r.RoleID, People: int32(r.People), ViaGroups: int32(r.ViaGroups),
			})
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.RoleUsageResponse)), nil
}

func (h *AuthzAdminHandler) AppUsage(ctx context.Context, req *connect.Request[anubisv1.AppUsageRequest]) (*connect.Response[anubisv1.AppUsageResponse], error) {
	out, err := h.f.Do(ctx, "admin.access.app_usage", func(ctx context.Context) (any, error) {
		r, err := h.svc.AppUsage(ctx, req.Msg.ApplicationId)
		if err != nil {
			return nil, err
		}
		return &anubisv1.AppUsageResponse{
			People: int32(r.People), Direct: int32(r.Direct), ViaGroups: int32(r.ViaGroups),
		}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.AppUsageResponse)), nil
}

func (h *AuthzAdminHandler) HolderKinds(ctx context.Context, req *connect.Request[anubisv1.HolderKindsRequest]) (*connect.Response[anubisv1.HolderKindsResponse], error) {
	out, err := h.f.Do(ctx, "admin.access.holder_kinds", func(ctx context.Context) (any, error) {
		r, err := h.svc.HolderKinds(ctx, req.Msg.RoleId, req.Msg.Axis)
		if err != nil {
			return nil, err
		}
		return &anubisv1.HolderKindsResponse{
			Total: int32(r.Total), Everything: int32(r.Everything), Branch: int32(r.Branch),
			Fixed: int32(r.Fixed), Nothing: int32(r.Nothing), Full: int32(r.Full),
		}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.HolderKindsResponse)), nil
}

func (h *AuthzAdminHandler) PlaceHolders(ctx context.Context, req *connect.Request[anubisv1.PlaceHoldersRequest]) (*connect.Response[anubisv1.PlaceHoldersResponse], error) {
	out, err := h.f.Do(ctx, "admin.access.place_holders", func(ctx context.Context) (any, error) {
		rows, err := h.svc.PlaceHolders(ctx, req.Msg.NodeId, int(req.Msg.Limit))
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.PlaceHoldersResponse{}
		for _, r := range rows {
			resp.Holders = append(resp.Holders, &anubisv1.PlaceHolder{
				IdentityId: r.IdentityID, Username: r.Username, RoleName: r.RoleName,
				GroupName: r.GroupName, OnlyHere: r.OnlyHere, FromAbove: r.FromAbove,
			})
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.PlaceHoldersResponse)), nil
}
