package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) ListGrants(ctx context.Context, tenantID, identityID string, includeRevoked bool) ([]repository.GrantRecord, error) {
	rows, err := s.q(ctx).ListGrantsByIdentity(ctx, gen.ListGrantsByIdentityParams{
		IdentityID: identityID, TenantID: tenantID, IncludeRevoked: includeRevoked,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.GrantRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.GrantRecord{
			ID: r.ID, IdentityID: r.IdentityID, RoleID: r.RoleID,
			RoleName: r.RoleName, SelfScoped: r.SelfScoped,
			ValidFrom: r.ValidFrom, ValidUntil: r.ValidUntil, RevokedAt: r.RevokedAt,
			GrantedBy: r.GrantedBy, ViaMembershipID: deref(r.ViaMembershipID),
			Reason: deref(r.Reason),
		})
	}
	return out, nil
}

func (s *Store) GrantScopes(ctx context.Context, grantIDs []string) ([]repository.GrantScopeRecord, error) {
	if len(grantIDs) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListGrantScopes(ctx, grantIDs)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.GrantScopeRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.GrantScopeRecord{
			GrantID: r.GrantID, Axis: r.AxisCode, NodeID: r.ScopeNodeID,
			NodeName: r.NodeName, Inherit: r.Inherit,
		})
	}
	return out, nil
}

func (s *Store) CreateGrant(ctx context.Context, in repository.GrantCreate) (string, error) {
	var id string
	err := s.WithinTx(ctx, func(ctx context.Context) error {
		row, err := s.q(ctx).CreateGrant(ctx, gen.CreateGrantParams{
			TenantID: in.TenantID, IdentityID: in.IdentityID, RoleID: in.RoleID,
			GrantedBy: in.GrantedBy, Reason: in.Reason,
			SelfScoped: in.SelfScoped, ValidUntil: optTime(in.ValidUntil),
		})
		if err != nil {
			return mapErr(err)
		}
		id = row.ID
		for _, sc := range in.Scopes {
			if err := s.q(ctx).InsertGrantScope(ctx, gen.InsertGrantScopeParams{
				GrantID: id, TenantID: in.TenantID, AxisCode: sc.Axis,
				ScopeNodeID: sc.NodeID, Inherit: sc.Inherit,
			}); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
	return id, err
}

func (s *Store) RevokeGrant(ctx context.Context, tenantID, id, reason string) error {
	_, err := s.q(ctx).RevokeGrant(ctx, gen.RevokeGrantParams{
		ID: id, TenantID: tenantID, Reason: reason,
	})
	if err != nil {
		return mapErr(err)
	}
	return nil
}

var _ = domain.ErrNotFound // keep import if RevokeGrant mapping changes
