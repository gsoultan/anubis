package authzpg

import (
	"context"

	authzrquery "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rquery"
	"github.com/gsoultan/anubis/internal/authz/domain/grant"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) ListGrants(ctx context.Context, tenantID, identityID string, includeRevoked bool) ([]grant.GrantRecord, error) {
	rows, err := authzrquery.ListGrantsByIdentity.Query(ctx, s.rex(ctx),
		identityID, tenantID, includeRevoked)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]grant.GrantRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, grant.GrantRecord{
			ID: r.ID, IdentityID: r.IdentityID, RoleID: r.RoleID,
			RoleName: r.RoleName, SelfScoped: r.SelfScoped,
			ValidFrom: r.ValidFrom, ValidUntil: r.ValidUntil.Ptr(), RevokedAt: r.RevokedAt.Ptr(),
			GrantedBy: r.GrantedBy, ViaMembershipID: r.ViaMembershipID.V,
			Reason: r.Reason.V,
		})
	}
	return out, nil
}

func (s *Repository) GrantScopes(ctx context.Context, grantIDs []string) ([]grant.GrantScopeRecord, error) {
	if len(grantIDs) == 0 {
		return nil, nil
	}
	rows, err := authzrquery.ListGrantScopes.Query(ctx, s.rex(ctx), grantIDs)
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
		row, ok, err := authzrquery.CreateGrant.One(ctx, s.rex(ctx),
			in.TenantID, in.IdentityID, in.RoleID, in.GrantedBy, in.Reason,
			in.SelfScoped, database.OptTime(in.ValidUntil))
		if err != nil {
			return database.MapErr(err)
		}
		if !ok {
			return apperr.ErrNotFound
		}
		id = row.ID
		for _, sc := range in.Scopes {
			if _, err := authzrquery.InsertGrantScope.Exec(ctx, s.rex(ctx),
				id, in.TenantID, sc.Axis, sc.NodeID, sc.Inherit); err != nil {
				return database.MapErr(err)
			}
		}
		return nil
	})
	return id, err
}

func (s *Repository) RevokeGrant(ctx context.Context, tenantID, id, reason string) error {
	// The sqlc form used :one but discarded the row; an already-revoked
	// grant produced ErrNoRows that the caller never saw. Preserved: only
	// real errors surface.
	_, _, err := authzrquery.RevokeGrant.One(ctx, s.rex(ctx), id, tenantID, reason)
	if err != nil {
		return database.MapErr(err)
	}
	return nil
}

// SearchGrants backs the Access screen: filters narrow first, keyset paging
// carries the rest. Ordered by (created_at, id) so the cursor stays stable
// when two grants share a timestamp — which they do, in bulk imports.
func (s *Repository) SearchGrants(ctx context.Context, tenantID string, q grant.GrantSearch) ([]grant.GrantHit, error) {
	size := q.PageSize
	if size <= 0 {
		size = 50
	}
	if size > 200 {
		size = 200
	}
	rows, err := authzrquery.SearchGrants.Query(ctx, s.rex(ctx),
		tenantID, q.IncludeRevoked, q.IdentityID, q.RoleID, q.Source,
		q.Query, q.Cursor, int32(size))
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]grant.GrantHit, 0, len(rows))
	for _, r := range rows {
		out = append(out, grant.GrantHit{
			Username: r.Username,
			Grant: grant.GrantRecord{
				ID: r.ID, IdentityID: r.IdentityID, RoleID: r.RoleID,
				RoleName: r.RoleName, SelfScoped: r.SelfScoped,
				ValidFrom: r.ValidFrom, ValidUntil: r.ValidUntil.Ptr(), RevokedAt: r.RevokedAt.Ptr(),
				GrantedBy: r.GrantedBy, ViaMembershipID: r.ViaMembershipID.V,
				Reason: r.Reason.V,
			},
		})
	}
	return out, nil
}

// CountLiveGrants backs the console's overview: access currently in force.
func (s *Repository) CountLiveGrants(ctx context.Context, tenantID string) (int64, error) {
	row, ok, err := authzrquery.CountLiveGrants.One(ctx, s.rex(ctx), tenantID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	if !ok {
		return 0, apperr.ErrNotFound
	}
	return row.Count, nil
}
