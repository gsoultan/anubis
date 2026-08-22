package authzpg

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/authz/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/authz/domain/grant"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) ListGrants(ctx context.Context, tenantID, identityID string, includeRevoked bool) ([]grant.GrantRecord, error) {
	rows, err := s.q(ctx).ListGrantsByIdentity(ctx, gen.ListGrantsByIdentityParams{
		IdentityID: identityID, TenantID: tenantID, IncludeRevoked: includeRevoked,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]grant.GrantRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, grant.GrantRecord{
			ID: r.ID, IdentityID: r.IdentityID, RoleID: r.RoleID,
			RoleName: r.RoleName, SelfScoped: r.SelfScoped,
			ValidFrom: r.ValidFrom, ValidUntil: r.ValidUntil, RevokedAt: r.RevokedAt,
			GrantedBy: r.GrantedBy, ViaMembershipID: database.Deref(r.ViaMembershipID),
			Reason: database.Deref(r.Reason),
		})
	}
	return out, nil
}

func (s *Repository) GrantScopes(ctx context.Context, grantIDs []string) ([]grant.GrantScopeRecord, error) {
	if len(grantIDs) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListGrantScopes(ctx, grantIDs)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]grant.GrantScopeRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, grant.GrantScopeRecord{
			GrantID: r.GrantID, Axis: r.AxisCode, NodeID: r.ScopeNodeID,
			NodeName: r.NodeName, Inherit: r.Inherit,
		})
	}
	return out, nil
}

func (s *Repository) CreateGrant(ctx context.Context, in grant.GrantCreate) (string, error) {
	var id string
	err := s.WithinTx(ctx, func(ctx context.Context) error {
		row, err := s.q(ctx).CreateGrant(ctx, gen.CreateGrantParams{
			TenantID: in.TenantID, IdentityID: in.IdentityID, RoleID: in.RoleID,
			GrantedBy: in.GrantedBy, Reason: in.Reason,
			SelfScoped: in.SelfScoped, ValidUntil: database.OptTime(in.ValidUntil),
		})
		if err != nil {
			return database.MapErr(err)
		}
		id = row.ID
		for _, sc := range in.Scopes {
			if err := s.q(ctx).InsertGrantScope(ctx, gen.InsertGrantScopeParams{
				GrantID: id, TenantID: in.TenantID, AxisCode: sc.Axis,
				ScopeNodeID: sc.NodeID, Inherit: sc.Inherit,
			}); err != nil {
				return database.MapErr(err)
			}
		}
		return nil
	})
	return id, err
}

func (s *Repository) RevokeGrant(ctx context.Context, tenantID, id, reason string) error {
	_, err := s.q(ctx).RevokeGrant(ctx, gen.RevokeGrantParams{
		ID: id, TenantID: tenantID, Reason: reason,
	})
	if err != nil {
		return database.MapErr(err)
	}
	return nil
}

var _ = apperr.ErrNotFound // keep import if RevokeGrant mapping changes
