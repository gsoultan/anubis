package connectrpc

import (
	"context"
	"time"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	ep "github.com/gsoultan/anubis/internal/endpoint"
	"github.com/gsoultan/anubis/internal/repository"
	"github.com/gsoultan/anubis/internal/service"
)

// AuthzAdminHandler implements anubisv1connect.AuthzAdminServiceHandler.
type AuthzAdminHandler struct {
	svc service.AuthzAdminService
	f   ep.Factory
}

func NewAuthzAdminHandler(svc service.AuthzAdminService, f ep.Factory) *AuthzAdminHandler {
	return &AuthzAdminHandler{svc: svc, f: f}
}

var _ anubisv1connect.AuthzAdminServiceHandler = (*AuthzAdminHandler)(nil)

func (h *AuthzAdminHandler) ListRoles(ctx context.Context, req *connect.Request[anubisv1.ListRolesRequest]) (*connect.Response[anubisv1.ListRolesResponse], error) {
	out, err := h.f.Do(ctx, "admin.role.list", func(ctx context.Context) (any, error) {
		roles, parents, patterns, err := h.svc.ListRoles(ctx, req.Msg.Query)
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.ListRolesResponse{}
		for _, r := range roles {
			resp.Roles = append(resp.Roles, &anubisv1.Role{
				Id: r.ID, Name: r.Name, Description: r.Description,
				ApplicationSlug: r.ApplicationSlug, IsSystem: r.IsSystem,
				AllowedRealmKinds: r.AllowedRealmKinds, AssignableAt: r.AssignableAt,
				ParentIds: parents[r.ID], Patterns: patterns[r.ID],
			})
		}
		return resp, nil
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ListRolesResponse)), nil
}

func roleRecord(r *anubisv1.Role) repository.RoleRecord {
	return repository.RoleRecord{
		ID: r.Id, Name: r.Name, Description: r.Description,
		ApplicationSlug:   r.ApplicationSlug,
		AllowedRealmKinds: r.AllowedRealmKinds, AssignableAt: r.AssignableAt,
	}
}

func roleProto(r *repository.RoleRecord, parents, patterns []string) *anubisv1.Role {
	return &anubisv1.Role{
		Id: r.ID, Name: r.Name, Description: r.Description,
		ApplicationSlug: r.ApplicationSlug, IsSystem: r.IsSystem,
		AllowedRealmKinds: r.AllowedRealmKinds, AssignableAt: r.AssignableAt,
		ParentIds: parents, Patterns: patterns,
	}
}

func (h *AuthzAdminHandler) CreateRole(ctx context.Context, req *connect.Request[anubisv1.CreateRoleRequest]) (*connect.Response[anubisv1.CreateRoleResponse], error) {
	out, err := h.f.Do(ctx, "admin.role.create", func(ctx context.Context) (any, error) {
		return h.svc.CreateRole(ctx, roleRecord(req.Msg.Role), req.Msg.Role.ParentIds, req.Msg.Role.Patterns)
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.CreateRoleResponse{
		Role: roleProto(out.(*repository.RoleRecord), req.Msg.Role.ParentIds, req.Msg.Role.Patterns),
	}), nil
}

func (h *AuthzAdminHandler) UpdateRole(ctx context.Context, req *connect.Request[anubisv1.UpdateRoleRequest]) (*connect.Response[anubisv1.UpdateRoleResponse], error) {
	out, err := h.f.Do(ctx, "admin.role.update", func(ctx context.Context) (any, error) {
		return h.svc.UpdateRole(ctx, roleRecord(req.Msg.Role), req.Msg.Role.ParentIds, req.Msg.Role.Patterns)
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.UpdateRoleResponse{
		Role: roleProto(out.(*repository.RoleRecord), req.Msg.Role.ParentIds, req.Msg.Role.Patterns),
	}), nil
}

func (h *AuthzAdminHandler) GetRoleEffective(ctx context.Context, req *connect.Request[anubisv1.GetRoleEffectiveRequest]) (*connect.Response[anubisv1.GetRoleEffectiveResponse], error) {
	out, err := h.f.Do(ctx, "admin.role.effective", func(ctx context.Context) (any, error) {
		return h.svc.GetRoleEffective(ctx, req.Msg.RoleId)
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	resp := &anubisv1.GetRoleEffectiveResponse{}
	for _, e := range out.([]repository.EffectivePermissionRecord) {
		resp.Permissions = append(resp.Permissions, &anubisv1.EffectivePermission{
			PermissionKey: e.Key, ViaRole: e.ViaRole,
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *AuthzAdminHandler) ListPermissions(ctx context.Context, req *connect.Request[anubisv1.ListPermissionsRequest]) (*connect.Response[anubisv1.ListPermissionsResponse], error) {
	out, err := h.f.Do(ctx, "admin.permission.list", func(ctx context.Context) (any, error) {
		return h.svc.ListPermissions(ctx, req.Msg.ApplicationSlug, req.Msg.IncludeDeprecated)
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	resp := &anubisv1.ListPermissionsResponse{}
	for _, p := range out.([]repository.PermissionRecord) {
		resp.Permissions = append(resp.Permissions, &anubisv1.Permission{
			Id: p.ID, Key: p.Key, AppSlug: p.AppSlug, Resource: p.Resource,
			Action: p.Action, Risk: p.Risk, Description: p.Description,
			MinAssurance: int32(p.MinAssurance), RequiresAmr: p.RequiresAMR,
			MaxAuthAge: p.MaxAuthAge, Deprecated: p.Deprecated,
		})
	}
	return connect.NewResponse(resp), nil
}

func grantScopeProtos(scopes []repository.GrantScopeRecord, grantID string) []*anubisv1.GrantScope {
	var out []*anubisv1.GrantScope
	for _, s := range scopes {
		if s.GrantID != grantID {
			continue
		}
		out = append(out, &anubisv1.GrantScope{
			Axis: s.Axis, NodeId: s.NodeID, NodeName: s.NodeName, Inherit: s.Inherit,
		})
	}
	return out
}

func (h *AuthzAdminHandler) ListGrants(ctx context.Context, req *connect.Request[anubisv1.ListGrantsRequest]) (*connect.Response[anubisv1.ListGrantsResponse], error) {
	out, err := h.f.Do(ctx, "admin.grant.list", func(ctx context.Context) (any, error) {
		grants, scopes, err := h.svc.ListGrants(ctx, req.Msg.IdentityId, req.Msg.IncludeRevoked)
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.ListGrantsResponse{}
		for _, g := range grants {
			pg := &anubisv1.Grant{
				Id: g.ID, IdentityId: g.IdentityID, RoleId: g.RoleID,
				RoleName: g.RoleName, SelfScoped: g.SelfScoped,
				ValidFrom: g.ValidFrom.Unix(), GrantedBy: g.GrantedBy,
				ViaMembershipId: g.ViaMembershipID, Reason: g.Reason,
				Scopes: grantScopeProtos(scopes, g.ID),
			}
			if g.ValidUntil != nil {
				pg.ValidUntil = g.ValidUntil.Unix()
			}
			if g.RevokedAt != nil {
				pg.RevokedAt = g.RevokedAt.Unix()
			}
			resp.Grants = append(resp.Grants, pg)
		}
		return resp, nil
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ListGrantsResponse)), nil
}

func (h *AuthzAdminHandler) CreateGrant(ctx context.Context, req *connect.Request[anubisv1.CreateGrantRequest]) (*connect.Response[anubisv1.CreateGrantResponse], error) {
	out, err := h.f.Do(ctx, "admin.grant.create", func(ctx context.Context) (any, error) {
		in := repository.GrantCreate{
			IdentityID: req.Msg.IdentityId, RoleID: req.Msg.RoleId,
			SelfScoped: req.Msg.SelfScoped, Reason: req.Msg.Reason,
		}
		if req.Msg.ValidUntil > 0 {
			t := time.Unix(req.Msg.ValidUntil, 0)
			in.ValidUntil = &t
		}
		for _, s := range req.Msg.Scopes {
			in.Scopes = append(in.Scopes, repository.GrantScopeInput{
				Axis: s.Axis, NodeID: s.NodeId, Inherit: s.Inherit,
			})
		}
		return h.svc.CreateGrant(ctx, in)
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.CreateGrantResponse{
		Grant: &anubisv1.Grant{Id: out.(string), IdentityId: req.Msg.IdentityId, RoleId: req.Msg.RoleId},
	}), nil
}

func (h *AuthzAdminHandler) RevokeGrant(ctx context.Context, req *connect.Request[anubisv1.RevokeGrantRequest]) (*connect.Response[anubisv1.RevokeGrantResponse], error) {
	if _, err := h.f.Do(ctx, "admin.grant.revoke", func(ctx context.Context) (any, error) {
		return nil, h.svc.RevokeGrant(ctx, req.Msg.GrantId, req.Msg.Reason)
	}); err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.RevokeGrantResponse{}), nil
}

func (h *AuthzAdminHandler) ListMemberships(ctx context.Context, _ *connect.Request[anubisv1.ListMembershipsRequest]) (*connect.Response[anubisv1.ListMembershipsResponse], error) {
	out, err := h.f.Do(ctx, "admin.membership.list", func(ctx context.Context) (any, error) {
		ms, entries, scopes, err := h.svc.ListMemberships(ctx)
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.ListMembershipsResponse{}
		for _, m := range ms {
			pm := &anubisv1.Membership{
				Id: m.ID, Name: m.Name, Description: m.Description,
				MemberCount: int32(m.MemberCount),
			}
			for _, e := range entries {
				if e.MembershipID != m.ID {
					continue
				}
				pe := &anubisv1.MembershipEntry{Id: e.ID, RoleId: e.RoleID, RoleName: e.RoleName}
				for _, s := range scopes {
					if s.EntryID == e.ID {
						pe.Scopes = append(pe.Scopes, &anubisv1.GrantScope{
							Axis: s.Axis, NodeId: s.NodeID, NodeName: s.NodeName, Inherit: s.Inherit,
						})
					}
				}
				pm.Entries = append(pm.Entries, pe)
			}
			resp.Memberships = append(resp.Memberships, pm)
		}
		return resp, nil
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ListMembershipsResponse)), nil
}

func (h *AuthzAdminHandler) CreateMembership(ctx context.Context, req *connect.Request[anubisv1.CreateMembershipRequest]) (*connect.Response[anubisv1.CreateMembershipResponse], error) {
	out, err := h.f.Do(ctx, "admin.membership.create", func(ctx context.Context) (any, error) {
		return h.svc.CreateMembership(ctx, req.Msg.Name, req.Msg.Description)
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	m := out.(*repository.MembershipRecord)
	return connect.NewResponse(&anubisv1.CreateMembershipResponse{
		Membership: &anubisv1.Membership{Id: m.ID, Name: m.Name, Description: m.Description},
	}), nil
}

func (h *AuthzAdminHandler) SetMembershipEntries(ctx context.Context, req *connect.Request[anubisv1.SetMembershipEntriesRequest]) (*connect.Response[anubisv1.SetMembershipEntriesResponse], error) {
	out, err := h.f.Do(ctx, "admin.membership.set_entries", func(ctx context.Context) (any, error) {
		entries := make([]repository.MembershipEntryInput, 0, len(req.Msg.Entries))
		for _, e := range req.Msg.Entries {
			in := repository.MembershipEntryInput{RoleID: e.RoleId}
			for _, s := range e.Scopes {
				in.Scopes = append(in.Scopes, repository.GrantScopeInput{
					Axis: s.Axis, NodeID: s.NodeId, Inherit: s.Inherit,
				})
			}
			entries = append(entries, in)
		}
		return h.svc.SetMembershipEntries(ctx, req.Msg.MembershipId, entries)
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.SetMembershipEntriesResponse{
		GrantsChanged: int32(out.(int)),
	}), nil
}

func (h *AuthzAdminHandler) AssignMembership(ctx context.Context, req *connect.Request[anubisv1.AssignMembershipRequest]) (*connect.Response[anubisv1.AssignMembershipResponse], error) {
	out, err := h.f.Do(ctx, "admin.membership.assign", func(ctx context.Context) (any, error) {
		return h.svc.AssignMembership(ctx, req.Msg.MembershipId, req.Msg.IdentityId)
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.AssignMembershipResponse{GrantsCreated: int32(out.(int))}), nil
}

func (h *AuthzAdminHandler) UnassignMembership(ctx context.Context, req *connect.Request[anubisv1.UnassignMembershipRequest]) (*connect.Response[anubisv1.UnassignMembershipResponse], error) {
	out, err := h.f.Do(ctx, "admin.membership.unassign", func(ctx context.Context) (any, error) {
		return h.svc.UnassignMembership(ctx, req.Msg.MembershipId, req.Msg.IdentityId)
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.UnassignMembershipResponse{GrantsRevoked: int32(out.(int))}), nil
}

func (h *AuthzAdminHandler) ResyncMembership(ctx context.Context, req *connect.Request[anubisv1.ResyncMembershipRequest]) (*connect.Response[anubisv1.ResyncMembershipResponse], error) {
	out, err := h.f.Do(ctx, "admin.membership.resync", func(ctx context.Context) (any, error) {
		return h.svc.ResyncMembership(ctx, req.Msg.MembershipId)
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(&anubisv1.ResyncMembershipResponse{GrantsChanged: int32(out.(int))}), nil
}

func (h *AuthzAdminHandler) ApplyManifest(ctx context.Context, req *connect.Request[anubisv1.ApplyManifestRequest]) (*connect.Response[anubisv1.ApplyManifestResponse], error) {
	out, err := h.f.Do(ctx, "admin.manifest.apply", func(ctx context.Context) (any, error) {
		report, version, err := h.svc.ApplyManifest(ctx, req.Msg.ApplicationSlug, req.Msg.ManifestJson, req.Msg.Dry)
		if err != nil {
			return nil, err
		}
		return &anubisv1.ApplyManifestResponse{ReportJson: report, ManifestVersion: int32(version)}, nil
	})
	if err != nil {
		return nil, toConnectErr(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ApplyManifestResponse)), nil
}
