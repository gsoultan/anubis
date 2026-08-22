package authzadmin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/authz/domain/grant"
	"github.com/gsoultan/anubis/internal/authz/domain/membership"
	"github.com/gsoultan/anubis/internal/authz/guard"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/txm"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// authzAdminInteractor implements AuthzAdminUsecase.
type authzAdminInteractor struct {
	guard   *guard.Guard
	roles   authzport.RoleRepository
	perms   authzport.PermissionCatalogRepository
	grants  authzport.GrantRepository
	members authzport.MembershipRepository
	apps    tenancyport.ApplicationRepository
	routes  tenancyport.RouteRepository
	tx      txm.TxManager
	audit   auditport.Auditor
}

func NewAuthzAdminInteractor(
	authz authzport.AuthzRepository,
	roles authzport.RoleRepository,
	perms authzport.PermissionCatalogRepository,
	grants authzport.GrantRepository,
	members authzport.MembershipRepository,
	apps tenancyport.ApplicationRepository,
	routes tenancyport.RouteRepository,
	tx txm.TxManager,
	audit auditport.Auditor,
) AuthzAdminUsecase {
	return &authzAdminInteractor{
		guard: guard.New(authz), roles: roles, perms: perms,
		grants: grants, members: members, apps: apps, routes: routes,
		tx: tx, audit: audit,
	}
}

func (u *authzAdminInteractor) emit(ctx context.Context, p *authctx.Principal, action, target string, detail map[string]string) {
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, TargetID: target, Action: action, Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: jsonx.Must(detail),
	})
}

func (u *authzAdminInteractor) ListRoles(ctx context.Context, query string) ([]authzdomain.RoleRecord, map[string][]string, map[string][]string, error) {
	p, err := u.guard.Require(ctx, "anubis:role:admin")
	if err != nil {
		return nil, nil, nil, err
	}
	roles, err := u.roles.ListRoles(ctx, p.TenantID, query)
	if err != nil {
		return nil, nil, nil, err
	}
	parents := make(map[string][]string, len(roles))
	patterns := make(map[string][]string, len(roles))
	for _, r := range roles {
		if ps, err := u.roles.RoleParents(ctx, r.ID); err == nil {
			parents[r.ID] = ps
		}
		if pt, err := u.roles.RolePatterns(ctx, r.ID); err == nil {
			patterns[r.ID] = pt
		}
	}
	return roles, parents, patterns, nil
}

func (u *authzAdminInteractor) CreateRole(ctx context.Context, r authzdomain.RoleRecord, parents, patterns []string) (*authzdomain.RoleRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:role:admin")
	if err != nil {
		return nil, err
	}
	appID := ""
	if r.ApplicationSlug != "" {
		app, aerr := u.apps.ApplicationBySlug(ctx, p.TenantID, r.ApplicationSlug)
		if aerr != nil {
			return nil, apperr.ErrInvalidArgument.With("application", r.ApplicationSlug)
		}
		appID = app.ID
	}
	var id string
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		var cerr error
		if id, cerr = u.roles.CreateRole(ctx, p.TenantID, r, appID); cerr != nil {
			return cerr
		}
		return u.applyRoleGraph(ctx, id, parents, patterns)
	})
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "role.create", id, map[string]string{"name": r.Name})
	return u.roles.RoleByID(ctx, p.TenantID, id)
}

func (u *authzAdminInteractor) UpdateRole(ctx context.Context, r authzdomain.RoleRecord, parents, patterns []string) (*authzdomain.RoleRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:role:admin")
	if err != nil {
		return nil, err
	}
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := u.roles.UpdateRole(ctx, p.TenantID, r); err != nil {
			return err
		}
		return u.applyRoleGraph(ctx, r.ID, parents, patterns)
	})
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "role.update", r.ID, map[string]string{"name": r.Name})
	return u.roles.RoleByID(ctx, p.TenantID, r.ID)
}

// applyRoleGraph replaces parents/patterns and recomputes the role plus every
// role that inherits from it — flatten at write, probe at read.
func (u *authzAdminInteractor) applyRoleGraph(ctx context.Context, roleID string, parents, patterns []string) error {
	if err := u.roles.SetRoleParents(ctx, roleID, parents); err != nil {
		return err
	}
	if err := u.roles.SetRolePatterns(ctx, roleID, patterns); err != nil {
		return err
	}
	if err := u.roles.RecomputeRole(ctx, roleID); err != nil {
		return err
	}
	below, err := u.roles.RolesBelow(ctx, roleID)
	if err != nil {
		return err
	}
	for _, rid := range below {
		if err := u.roles.RecomputeRole(ctx, rid); err != nil {
			return err
		}
	}
	return nil
}

func (u *authzAdminInteractor) GetRoleEffective(ctx context.Context, roleID string) ([]authzdomain.EffectivePermissionRecord, error) {
	if _, err := u.guard.Require(ctx, "anubis:role:admin"); err != nil {
		return nil, err
	}
	return u.roles.RoleEffective(ctx, roleID)
}

func (u *authzAdminInteractor) ListPermissions(ctx context.Context, applicationSlug string, includeDeprecated bool) ([]authzdomain.PermissionRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	appID := ""
	if applicationSlug != "" {
		app, aerr := u.apps.ApplicationBySlug(ctx, p.TenantID, applicationSlug)
		if aerr != nil {
			return nil, apperr.ErrNotFound.With("application", applicationSlug)
		}
		appID = app.ID
	}
	return u.perms.ListPermissions(ctx, p.TenantID, appID, includeDeprecated)
}

func (u *authzAdminInteractor) ListGrants(ctx context.Context, identityID string, includeRevoked bool) ([]grant.GrantRecord, []grant.GrantScopeRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, nil, err
	}
	grants, err := u.grants.ListGrants(ctx, p.TenantID, identityID, includeRevoked)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]string, len(grants))
	for i, g := range grants {
		ids[i] = g.ID
	}
	scopes, err := u.grants.GrantScopes(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	return grants, scopes, nil
}

func (u *authzAdminInteractor) CreateGrant(ctx context.Context, in grant.GrantCreate) (string, error) {
	p, err := u.guard.Require(ctx, "anubis:grant:admin")
	if err != nil {
		return "", err
	}
	in.TenantID = p.TenantID
	in.GrantedBy = p.IdentityID
	// The schema is the enforcement layer here: realm-kind guard,
	// role_grantable, cross-tenant impossibility, self-scope purity all fire
	// as constraint triggers on this insert.
	id, err := u.grants.CreateGrant(ctx, in)
	if err != nil {
		return "", err
	}
	u.emit(ctx, p, "grant.create", in.IdentityID, map[string]string{"grant_id": id, "role_id": in.RoleID})
	return id, nil
}

func (u *authzAdminInteractor) RevokeGrant(ctx context.Context, grantID, reason string) error {
	p, err := u.guard.Require(ctx, "anubis:grant:admin")
	if err != nil {
		return err
	}
	if err := u.grants.RevokeGrant(ctx, p.TenantID, grantID, reason); err != nil {
		return err
	}
	u.emit(ctx, p, "grant.revoke", grantID, map[string]string{"reason": reason})
	return nil
}

func (u *authzAdminInteractor) ListMemberships(ctx context.Context) ([]membership.MembershipRecord, []membership.MembershipEntryRecord, []membership.MembershipEntryScopeRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:membership:admin")
	if err != nil {
		return nil, nil, nil, err
	}
	ms, err := u.members.ListMemberships(ctx, p.TenantID)
	if err != nil {
		return nil, nil, nil, err
	}
	mids := make([]string, len(ms))
	for i, m := range ms {
		mids[i] = m.ID
	}
	entries, err := u.members.MembershipEntries(ctx, mids)
	if err != nil {
		return nil, nil, nil, err
	}
	eids := make([]string, len(entries))
	for i, e := range entries {
		eids[i] = e.ID
	}
	scopes, err := u.members.MembershipEntryScopes(ctx, eids)
	if err != nil {
		return nil, nil, nil, err
	}
	return ms, entries, scopes, nil
}

func (u *authzAdminInteractor) CreateMembership(ctx context.Context, name, description string) (*membership.MembershipRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:membership:admin")
	if err != nil {
		return nil, err
	}
	id, err := u.members.CreateMembership(ctx, p.TenantID, name, description)
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "membership.create", id, map[string]string{"name": name})
	return &membership.MembershipRecord{ID: id, Name: name, Description: description}, nil
}

func (u *authzAdminInteractor) SetMembershipEntries(ctx context.Context, membershipID string, entries []membership.MembershipEntryInput) (int, error) {
	p, err := u.guard.Require(ctx, "anubis:membership:admin")
	if err != nil {
		return 0, err
	}
	var changed int
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := u.members.ReplaceMembershipEntries(ctx, p.TenantID, membershipID, entries); err != nil {
			return err
		}
		var rerr error
		changed, rerr = u.members.ResyncMembership(ctx, membershipID)
		return rerr
	})
	if err != nil {
		return 0, err
	}
	u.emit(ctx, p, "membership.set_entries", membershipID, map[string]string{"changed": fmt.Sprint(changed)})
	return changed, nil
}

func (u *authzAdminInteractor) AssignMembership(ctx context.Context, membershipID, identityID string) (int, error) {
	p, err := u.guard.Require(ctx, "anubis:grant:admin")
	if err != nil {
		return 0, err
	}
	n, err := u.members.AssignMembership(ctx, identityID, membershipID, p.IdentityID)
	if err != nil {
		return 0, err
	}
	u.emit(ctx, p, "membership.assign", identityID, map[string]string{"membership_id": membershipID})
	return n, nil
}

func (u *authzAdminInteractor) UnassignMembership(ctx context.Context, membershipID, identityID string) (int, error) {
	p, err := u.guard.Require(ctx, "anubis:grant:admin")
	if err != nil {
		return 0, err
	}
	n, err := u.members.UnassignMembership(ctx, identityID, membershipID)
	if err != nil {
		return 0, err
	}
	u.emit(ctx, p, "membership.unassign", identityID, map[string]string{"membership_id": membershipID})
	return n, nil
}

func (u *authzAdminInteractor) ResyncMembership(ctx context.Context, membershipID string) (int, error) {
	p, err := u.guard.Require(ctx, "anubis:membership:admin")
	if err != nil {
		return 0, err
	}
	n, err := u.members.ResyncMembership(ctx, membershipID)
	if err == nil {
		u.emit(ctx, p, "membership.resync", membershipID, nil)
	}
	return n, err
}

// ApplyManifest is registration-by-manifest: validate, diff, apply.
func (u *authzAdminInteractor) ApplyManifest(ctx context.Context, applicationSlug, manifestJSON string, dry bool) (string, int, error) {
	p, err := u.guard.Require(ctx, "anubis:manifest:apply")
	if err != nil {
		return "", 0, err
	}
	app, err := u.apps.ApplicationBySlug(ctx, p.TenantID, applicationSlug)
	if err != nil {
		return "", 0, apperr.ErrNotFound.With("application", applicationSlug)
	}
	var m Manifest
	if err := json.Unmarshal([]byte(manifestJSON), &m); err != nil {
		return "", 0, apperr.ErrInvalidArgument.With("manifest", "invalid JSON").Wrap(err)
	}
	if err := validateManifest(&m); err != nil {
		return "", 0, err
	}
	report := map[string]any{"dry": dry}
	version := app.ManifestVersion

	apply := func(ctx context.Context) error {
		// Permissions: upsert everything named; deprecate the rest.
		keepIDs := make([]string, 0, len(m.Permissions))
		keyByRA := make(map[string]string, len(m.Permissions))
		added, updated := 0, 0
		for _, mp := range m.Permissions {
			id, key, uerr := u.perms.UpsertPermission(ctx, p.TenantID, app.ID, app.Slug,
				authzdomain.PermissionRecord{
					Resource: mp.Resource, Action: mp.Action, Description: mp.Description,
					Risk: mp.Risk, MinAssurance: mp.MinAssurance,
					RequiresAMR: mp.RequiresAMR, MaxAuthAge: mp.MaxAuthAge,
				})
			if uerr != nil {
				return uerr
			}
			keepIDs = append(keepIDs, id)
			keyByRA[mp.Resource+":"+mp.Action] = id
			_ = key
			updated++
		}
		deprecated, derr := u.perms.DeprecatePermissionsExcept(ctx, app.ID, keepIDs)
		if derr != nil {
			return derr
		}
		report["permissions"] = map[string]any{
			"applied": updated, "added_or_updated": added + updated, "deprecated": deprecated,
		}

		// Roles: manifest roles are system roles named app.role.
		rolesApplied := 0
		for _, mr := range m.Roles {
			name := app.Slug + "." + mr.Name
			role, rerr := u.roles.RoleByName(ctx, p.TenantID, name)
			var roleID string
			if rerr != nil {
				roleID, rerr = u.roles.CreateRole(ctx, p.TenantID, authzdomain.RoleRecord{
					Name: name, Description: mr.Description, IsSystem: true,
					AllowedRealmKinds: mr.AllowedRealmKinds,
				}, app.ID)
				if rerr != nil {
					return rerr
				}
			} else {
				roleID = role.ID
			}
			permIDs := make([]string, 0, len(mr.Permissions))
			for _, ra := range mr.Permissions {
				id, ok := keyByRA[ra]
				if !ok {
					return apperr.ErrInvalidArgument.
						With("role", mr.Name).With("permission", ra)
				}
				permIDs = append(permIDs, id)
			}
			if err := u.roles.SetRolePermissions(ctx, roleID, permIDs); err != nil {
				return err
			}
			if err := u.applyRoleGraph(ctx, roleID, nil, mr.Patterns); err != nil {
				return err
			}
			rolesApplied++
		}
		report["roles"] = map[string]any{"applied": rolesApplied}

		// Routes: full replacement, priority-unique, shadow-checked.
		policies := make([]tenancydomain.RoutePolicyInput, 0, len(m.Routes))
		for _, rt := range m.Routes {
			permID := ""
			if rt.Effect == "require_permission" {
				id, ok := keyByRA[rt.Permission]
				if !ok {
					return apperr.ErrInvalidArgument.
						With("route", rt.PathPattern).With("permission", rt.Permission)
				}
				permID = id
			}
			bindings, _ := json.Marshal(rt.ScopeBindings)
			methods := rt.Methods
			if len(methods) == 0 {
				methods = []string{"*"}
			}
			policies = append(policies, tenancydomain.RoutePolicyInput{
				Priority: rt.Priority, Effect: rt.Effect, PathPattern: rt.PathPattern,
				HostPattern: rt.HostPattern, Methods: methods,
				PermissionID: permID, ScopeBindings: bindings,
			})
		}
		if err := u.routes.ReplaceRoutePolicies(ctx, p.TenantID, app.ID, policies); err != nil {
			return err
		}
		report["routes"] = map[string]any{"replaced": len(policies)}

		v, verr := u.apps.BumpManifestVersion(ctx, app.ID)
		if verr != nil {
			return verr
		}
		version = v
		return nil
	}

	if dry {
		// Dry run rides a transaction that always rolls back.
		sentinel := fmt.Errorf("manifest dry run rollback")
		if err := u.tx.WithinTx(ctx, func(ctx context.Context) error {
			if aerr := apply(ctx); aerr != nil {
				return aerr
			}
			return sentinel
		}); err != nil && !strings.Contains(err.Error(), sentinel.Error()) {
			return "", 0, err
		}
	} else {
		if err := u.tx.WithinTx(ctx, apply); err != nil {
			return "", 0, err
		}
		u.emit(ctx, p, "manifest.apply", app.ID, map[string]string{
			"application": applicationSlug, "version": fmt.Sprint(version),
		})
	}
	raw, _ := json.Marshal(report)
	return string(raw), version, nil
}

func validateManifest(m *Manifest) error {
	seen := map[string]bool{}
	for _, p := range m.Permissions {
		if p.Resource == "" || p.Action == "" {
			return apperr.ErrInvalidArgument.With("manifest", "permission missing resource/action")
		}
		seen[p.Resource+":"+p.Action] = true
	}
	prio := map[int]string{}
	for _, r := range m.Routes {
		switch r.Effect {
		case "public", "require_auth", "require_permission", "deny":
		default:
			return apperr.ErrInvalidArgument.With("route", r.PathPattern).With("effect", r.Effect)
		}
		if prev, dup := prio[r.Priority]; dup {
			return apperr.ErrInvalidArgument.With("route", r.PathPattern).
				With("conflict", prev).With("priority", fmt.Sprint(r.Priority))
		}
		prio[r.Priority] = r.PathPattern
	}
	return nil
}
