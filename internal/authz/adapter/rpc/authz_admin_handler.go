package authzrpc

import (
	"context"
	"time"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
	apiconnect "github.com/gsoultan/anubis/internal/api/connect"
	authzcatalog "github.com/gsoultan/anubis/internal/authz/app/catalog"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/authz/domain/catalogsync"
	"github.com/gsoultan/anubis/internal/authz/domain/grant"
	"github.com/gsoultan/anubis/internal/authz/domain/membership"
	authzsvc "github.com/gsoultan/anubis/internal/authz/service"
	"github.com/gsoultan/anubis/internal/platform/mw"
)

// AuthzAdminHandler implements anubisv1connect.AuthzAdminServiceHandler.
type AuthzAdminHandler struct {
	svc     authzsvc.AuthzAdminService
	catalog authzsvc.CatalogAdminService
	f       mw.Factory
}

func NewAuthzAdminHandler(svc authzsvc.AuthzAdminService, catalog authzsvc.CatalogAdminService, f mw.Factory) *AuthzAdminHandler {
	return &AuthzAdminHandler{svc: svc, catalog: catalog, f: f}
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
				Deprecated: r.Deprecated,
			})
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ListRolesResponse)), nil
}

func roleRecord(r *anubisv1.Role) authzdomain.RoleRecord {
	return authzdomain.RoleRecord{
		ID: r.Id, Name: r.Name, Description: r.Description,
		ApplicationSlug:   r.ApplicationSlug,
		AllowedRealmKinds: r.AllowedRealmKinds, AssignableAt: r.AssignableAt,
	}
}

func roleProto(r *authzdomain.RoleRecord, parents, patterns []string) *anubisv1.Role {
	return &anubisv1.Role{
		Id: r.ID, Name: r.Name, Description: r.Description,
		ApplicationSlug: r.ApplicationSlug, IsSystem: r.IsSystem,
		AllowedRealmKinds: r.AllowedRealmKinds, AssignableAt: r.AssignableAt,
		ParentIds: parents, Patterns: patterns, Deprecated: r.Deprecated,
	}
}

func (h *AuthzAdminHandler) CreateRole(ctx context.Context, req *connect.Request[anubisv1.CreateRoleRequest]) (*connect.Response[anubisv1.CreateRoleResponse], error) {
	out, err := h.f.Do(ctx, "admin.role.create", func(ctx context.Context) (any, error) {
		return h.svc.CreateRole(ctx, roleRecord(req.Msg.Role), req.Msg.Role.ParentIds, req.Msg.Role.Patterns)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.CreateRoleResponse{
		Role: roleProto(out.(*authzdomain.RoleRecord), req.Msg.Role.ParentIds, req.Msg.Role.Patterns),
	}), nil
}

func (h *AuthzAdminHandler) UpdateRole(ctx context.Context, req *connect.Request[anubisv1.UpdateRoleRequest]) (*connect.Response[anubisv1.UpdateRoleResponse], error) {
	out, err := h.f.Do(ctx, "admin.role.update", func(ctx context.Context) (any, error) {
		return h.svc.UpdateRole(ctx, roleRecord(req.Msg.Role), req.Msg.Role.ParentIds, req.Msg.Role.Patterns)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.UpdateRoleResponse{
		Role: roleProto(out.(*authzdomain.RoleRecord), req.Msg.Role.ParentIds, req.Msg.Role.Patterns),
	}), nil
}

func (h *AuthzAdminHandler) GetRoleEffective(ctx context.Context, req *connect.Request[anubisv1.GetRoleEffectiveRequest]) (*connect.Response[anubisv1.GetRoleEffectiveResponse], error) {
	out, err := h.f.Do(ctx, "admin.role.effective", func(ctx context.Context) (any, error) {
		return h.svc.GetRoleEffective(ctx, req.Msg.RoleId)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.GetRoleEffectiveResponse{}
	for _, e := range out.([]authzdomain.EffectivePermissionRecord) {
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
		return nil, apiconnect.Err(ctx, err)
	}
	resp := &anubisv1.ListPermissionsResponse{}
	for _, p := range out.([]authzdomain.PermissionRecord) {
		resp.Permissions = append(resp.Permissions, &anubisv1.Permission{
			Id: p.ID, Key: p.Key, AppSlug: p.AppSlug, Resource: p.Resource,
			Action: p.Action, Risk: p.Risk, Description: p.Description,
			MinAssurance: int32(p.MinAssurance), RequiresAmr: p.RequiresAMR,
			MaxAuthAge: p.MaxAuthAge, Deprecated: p.Deprecated,
		})
	}
	return connect.NewResponse(resp), nil
}

// scopeProto is the ONE place a GrantScope message is built, and scopeInputs
// below the ONE place one is read.
//
// Sharing only the outbound half is not sharing. That is precisely how 0045's
// interval_seconds shipped on list and went missing on create: the outbound
// mapping was factored out with a comment explaining why, and every call site
// still wrote the inbound direction by hand. The failure was silent — no
// error, no log, a source that simply never ran.
//
// A dropped `exclude` is the same shape and worse. The carve-out disappears,
// the grant is written WIDER than the operator asked for, and the only thing
// that would have said so is the screen they just left. Both directions are
// shared here, and scope_mapping_test.go fails if either drops a field.
func scopeProto(axis, nodeID, nodeName string, inherit, exclude bool) *anubisv1.GrantScope {
	return &anubisv1.GrantScope{
		Axis: axis, NodeId: nodeID, NodeName: nodeName,
		Inherit: inherit, Exclude: exclude,
	}
}

func scopeInputs(scopes []*anubisv1.GrantScope) []grant.GrantScopeInput {
	out := make([]grant.GrantScopeInput, 0, len(scopes))
	for _, s := range scopes {
		out = append(out, grant.GrantScopeInput{
			Axis: s.Axis, NodeID: s.NodeId, Inherit: s.Inherit, Exclude: s.Exclude,
		})
	}
	return out
}

func grantScopeProtos(scopes []grant.GrantScopeRecord, grantID string) []*anubisv1.GrantScope {
	var out []*anubisv1.GrantScope
	for _, s := range scopes {
		if s.GrantID != grantID {
			continue
		}
		out = append(out, scopeProto(s.Axis, s.NodeID, s.NodeName, s.Inherit, s.Exclude))
	}
	return out
}

func entryScopeProtos(scopes []membership.MembershipEntryScopeRecord, entryID string) []*anubisv1.GrantScope {
	var out []*anubisv1.GrantScope
	for _, s := range scopes {
		if s.EntryID != entryID {
			continue
		}
		out = append(out, scopeProto(s.Axis, s.NodeID, s.NodeName, s.Inherit, s.Exclude))
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
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ListGrantsResponse)), nil
}

func (h *AuthzAdminHandler) CreateGrant(ctx context.Context, req *connect.Request[anubisv1.CreateGrantRequest]) (*connect.Response[anubisv1.CreateGrantResponse], error) {
	out, err := h.f.Do(ctx, "admin.grant.create", func(ctx context.Context) (any, error) {
		in := grant.GrantCreate{
			IdentityID: req.Msg.IdentityId, RoleID: req.Msg.RoleId,
			SelfScoped: req.Msg.SelfScoped, Reason: req.Msg.Reason,
		}
		if req.Msg.ValidUntil > 0 {
			t := time.Unix(req.Msg.ValidUntil, 0)
			in.ValidUntil = &t
		}
		in.Scopes = scopeInputs(req.Msg.Scopes)
		return h.svc.CreateGrant(ctx, in)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.CreateGrantResponse{
		Grant: &anubisv1.Grant{Id: out.(string), IdentityId: req.Msg.IdentityId, RoleId: req.Msg.RoleId},
	}), nil
}

func (h *AuthzAdminHandler) RevokeGrant(ctx context.Context, req *connect.Request[anubisv1.RevokeGrantRequest]) (*connect.Response[anubisv1.RevokeGrantResponse], error) {
	if _, err := h.f.Do(ctx, "admin.grant.revoke", func(ctx context.Context) (any, error) {
		return nil, h.svc.RevokeGrant(ctx, req.Msg.GrantId, req.Msg.Reason)
	}); err != nil {
		return nil, apiconnect.Err(ctx, err)
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
				pm.Entries = append(pm.Entries, &anubisv1.MembershipEntry{
					Id: e.ID, RoleId: e.RoleID, RoleName: e.RoleName,
					Scopes: entryScopeProtos(scopes, e.ID),
				})
			}
			resp.Memberships = append(resp.Memberships, pm)
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ListMembershipsResponse)), nil
}

func (h *AuthzAdminHandler) CreateMembership(ctx context.Context, req *connect.Request[anubisv1.CreateMembershipRequest]) (*connect.Response[anubisv1.CreateMembershipResponse], error) {
	out, err := h.f.Do(ctx, "admin.membership.create", func(ctx context.Context) (any, error) {
		return h.svc.CreateMembership(ctx, req.Msg.Name, req.Msg.Description)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	m := out.(*membership.MembershipRecord)
	return connect.NewResponse(&anubisv1.CreateMembershipResponse{
		Membership: &anubisv1.Membership{Id: m.ID, Name: m.Name, Description: m.Description},
	}), nil
}

func (h *AuthzAdminHandler) SetMembershipEntries(ctx context.Context, req *connect.Request[anubisv1.SetMembershipEntriesRequest]) (*connect.Response[anubisv1.SetMembershipEntriesResponse], error) {
	out, err := h.f.Do(ctx, "admin.membership.set_entries", func(ctx context.Context) (any, error) {
		entries := make([]membership.MembershipEntryInput, 0, len(req.Msg.Entries))
		for _, e := range req.Msg.Entries {
			entries = append(entries, membership.MembershipEntryInput{
				RoleID: e.RoleId, Scopes: scopeInputs(e.Scopes),
			})
		}
		return h.svc.SetMembershipEntries(ctx, req.Msg.MembershipId, entries)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
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
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.AssignMembershipResponse{GrantsCreated: int32(out.(int))}), nil
}

func (h *AuthzAdminHandler) UnassignMembership(ctx context.Context, req *connect.Request[anubisv1.UnassignMembershipRequest]) (*connect.Response[anubisv1.UnassignMembershipResponse], error) {
	out, err := h.f.Do(ctx, "admin.membership.unassign", func(ctx context.Context) (any, error) {
		return h.svc.UnassignMembership(ctx, req.Msg.MembershipId, req.Msg.IdentityId)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.UnassignMembershipResponse{GrantsRevoked: int32(out.(int))}), nil
}

func (h *AuthzAdminHandler) ResyncMembership(ctx context.Context, req *connect.Request[anubisv1.ResyncMembershipRequest]) (*connect.Response[anubisv1.ResyncMembershipResponse], error) {
	out, err := h.f.Do(ctx, "admin.membership.resync", func(ctx context.Context) (any, error) {
		return h.svc.ResyncMembership(ctx, req.Msg.MembershipId)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.ResyncMembershipResponse{GrantsChanged: int32(out.(int))}), nil
}

func (h *AuthzAdminHandler) ApplyManifest(ctx context.Context, req *connect.Request[anubisv1.ApplyManifestRequest]) (*connect.Response[anubisv1.ApplyManifestResponse], error) {
	out, err := h.f.Do(ctx, "admin.manifest.apply", func(ctx context.Context) (any, error) {
		report, version, err := h.svc.ApplyManifest(ctx, req.Msg.ApplicationSlug, req.Msg.ManifestJson, req.Msg.Format, req.Msg.Dry)
		if err != nil {
			return nil, err
		}
		return &anubisv1.ApplyManifestResponse{ReportJson: report, ManifestVersion: int32(version)}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ApplyManifestResponse)), nil
}

func (h *AuthzAdminHandler) SearchGrants(ctx context.Context, req *connect.Request[anubisv1.SearchGrantsRequest]) (*connect.Response[anubisv1.SearchGrantsResponse], error) {
	out, err := h.f.Do(ctx, "admin.grant.search", func(ctx context.Context) (any, error) {
		hits, scopes, serr := h.svc.SearchGrants(ctx, grant.GrantSearch{
			Query:          req.Msg.Query,
			IdentityID:     req.Msg.IdentityId,
			RoleID:         req.Msg.RoleId,
			Source:         req.Msg.Source,
			IncludeRevoked: req.Msg.IncludeRevoked,
			Cursor:         req.Msg.PageToken,
			PageSize:       int(req.Msg.PageSize),
		})
		if serr != nil {
			return nil, serr
		}
		resp := &anubisv1.SearchGrantsResponse{}
		for _, hit := range hits {
			g := hit.Grant
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
			resp.Usernames = append(resp.Usernames, hit.Username)
		}
		// The cursor is the last row's id; an empty one means this was the
		// last page.
		if n := len(resp.Grants); n > 0 && n == int(req.Msg.PageSize) {
			resp.NextPageToken = resp.Grants[n-1].Id
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.SearchGrantsResponse)), nil
}

// --- catalog sources -------------------------------------------------------
//
// The catalog's fourth channel. The first three (an API call, an uploaded
// file, a role edited by hand) all arrive through ApplyManifest above and
// need none of this.

func catalogSourcePB(s catalogsync.Source) *anubisv1.CatalogSource {
	out := &anubisv1.CatalogSource{
		Id: s.ID, ApplicationSlug: s.ApplicationSlug, Name: s.Name,
		Kind: s.Kind, Format: s.Format, Status: s.Status,
		ConfigJson: string(s.Config), IntervalSeconds: int32(s.IntervalSeconds),
		LastStatus: s.LastStatus,
	}
	if s.LastRunAt != nil {
		out.LastRunAt = s.LastRunAt.Unix()
	}
	if s.NextRunAt != nil {
		out.NextRunAt = s.NextRunAt.Unix()
	}
	return out
}

func catalogRunPB(r catalogsync.Run) *anubisv1.CatalogRun {
	out := &anubisv1.CatalogRun{
		Id: r.ID, SourceId: r.SourceID, StartedAt: r.StartedAt.Unix(),
		Dry: r.Dry, Status: r.Status, Actor: r.Actor,
		DocumentSha: r.DocumentSHA, ReportJson: r.Report, Error: r.Error,
	}
	if r.FinishedAt != nil {
		out.FinishedAt = r.FinishedAt.Unix()
	}
	return out
}

func (h *AuthzAdminHandler) ListCatalogSources(ctx context.Context, _ *connect.Request[anubisv1.ListCatalogSourcesRequest]) (*connect.Response[anubisv1.ListCatalogSourcesResponse], error) {
	out, err := h.f.Do(ctx, "admin.catalog.sources", func(ctx context.Context) (any, error) {
		sources, err := h.catalog.ListSources(ctx)
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.ListCatalogSourcesResponse{}
		for _, s := range sources {
			resp.Sources = append(resp.Sources, catalogSourcePB(s))
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ListCatalogSourcesResponse)), nil
}

func (h *AuthzAdminHandler) CreateCatalogSource(ctx context.Context, req *connect.Request[anubisv1.CreateCatalogSourceRequest]) (*connect.Response[anubisv1.CreateCatalogSourceResponse], error) {
	out, err := h.f.Do(ctx, "admin.catalog.source.create", func(ctx context.Context) (any, error) {
		s, err := h.catalog.CreateSource(ctx, authzcatalog.SourceInput{
			ApplicationSlug: req.Msg.ApplicationSlug, Name: req.Msg.Name,
			Kind: req.Msg.Kind, Format: req.Msg.Format,
			ConfigJSON:      req.Msg.ConfigJson,
			IntervalSeconds: int(req.Msg.IntervalSeconds),
		})
		if err != nil {
			return nil, err
		}
		return &anubisv1.CreateCatalogSourceResponse{Source: catalogSourcePB(*s)}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.CreateCatalogSourceResponse)), nil
}

func (h *AuthzAdminHandler) UpdateCatalogSource(ctx context.Context, req *connect.Request[anubisv1.UpdateCatalogSourceRequest]) (*connect.Response[anubisv1.UpdateCatalogSourceResponse], error) {
	out, err := h.f.Do(ctx, "admin.catalog.source.update", func(ctx context.Context) (any, error) {
		s, err := h.catalog.UpdateSource(ctx, authzcatalog.SourceInput{
			ID: req.Msg.Id, Name: req.Msg.Name, Status: req.Msg.Status,
			Format:     req.Msg.Format,
			ConfigJSON: req.Msg.ConfigJson, IntervalSeconds: int(req.Msg.IntervalSeconds),
		})
		if err != nil {
			return nil, err
		}
		return &anubisv1.UpdateCatalogSourceResponse{Source: catalogSourcePB(*s)}, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.UpdateCatalogSourceResponse)), nil
}

func (h *AuthzAdminHandler) DeleteCatalogSource(ctx context.Context, req *connect.Request[anubisv1.DeleteCatalogSourceRequest]) (*connect.Response[anubisv1.DeleteCatalogSourceResponse], error) {
	_, err := h.f.Do(ctx, "admin.catalog.source.delete", func(ctx context.Context) (any, error) {
		return nil, h.catalog.DeleteSource(ctx, req.Msg.Id)
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(&anubisv1.DeleteCatalogSourceResponse{}), nil
}

func (h *AuthzAdminHandler) RunCatalogSource(ctx context.Context, req *connect.Request[anubisv1.RunCatalogSourceRequest]) (*connect.Response[anubisv1.RunCatalogSourceResponse], error) {
	out, err := h.f.Do(ctx, "admin.catalog.source.run", func(ctx context.Context) (any, error) {
		run, err := h.catalog.RunSource(ctx, req.Msg.SourceId, req.Msg.Dry)
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.RunCatalogSourceResponse{}
		if run != nil {
			resp.Run = catalogRunPB(*run)
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.RunCatalogSourceResponse)), nil
}

func (h *AuthzAdminHandler) ListCatalogRuns(ctx context.Context, req *connect.Request[anubisv1.ListCatalogRunsRequest]) (*connect.Response[anubisv1.ListCatalogRunsResponse], error) {
	out, err := h.f.Do(ctx, "admin.catalog.runs", func(ctx context.Context) (any, error) {
		runs, err := h.catalog.ListRuns(ctx, req.Msg.SourceId, req.Msg.Limit)
		if err != nil {
			return nil, err
		}
		resp := &anubisv1.ListCatalogRunsResponse{}
		for _, r := range runs {
			resp.Runs = append(resp.Runs, catalogRunPB(r))
		}
		return resp, nil
	})
	if err != nil {
		return nil, apiconnect.Err(ctx, err)
	}
	return connect.NewResponse(out.(*anubisv1.ListCatalogRunsResponse)), nil
}
