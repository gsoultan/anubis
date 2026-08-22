package authpg

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/auth/adapter/postgres/gen"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) CreateRefresh(ctx context.Context, in authdomain.RefreshInput) (string, error) {
	id, err := s.q(ctx).CreateRefreshToken(ctx, gen.CreateRefreshTokenParams{
		SessionID: in.SessionID, TenantID: in.TenantID, FamilyID: in.FamilyID,
		Generation: int32(in.Generation), TokenHash: in.TokenHash,
		ExpiresAt: in.ExpiresAt, BoundKey: in.BoundKey,
	})
	return id, database.MapErr(err)
}

func (s *Repository) ClaimRefresh(ctx context.Context, hash []byte) (*authdomain.RefreshClaim, error) {
	row, err := s.q(ctx).ClaimRefreshToken(ctx, hash)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &authdomain.RefreshClaim{
		ID: row.ID, SessionID: row.SessionID, TenantID: row.TenantID,
		FamilyID: row.FamilyID, Generation: int(row.Generation), ExpiresAt: row.ExpiresAt,
	}, nil
}

func (s *Repository) RefreshByHash(ctx context.Context, hash []byte) (*authdomain.RefreshInfo, error) {
	row, err := s.q(ctx).GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &authdomain.RefreshInfo{
		ID: row.ID, SessionID: row.SessionID, TenantID: row.TenantID,
		FamilyID: row.FamilyID, Status: row.Status, ExpiresAt: row.ExpiresAt,
	}, nil
}

func (s *Repository) SetRefreshSuccessor(ctx context.Context, id string, expiresAt time.Time, successorID string) error {
	return database.MapErr(s.q(ctx).SetRefreshSuccessor(ctx, gen.SetRefreshSuccessorParams{
		ID: id, ExpiresAt: expiresAt, SuccessorID: database.OptStr(successorID),
	}))
}

func (s *Repository) RevokeRefreshFamily(ctx context.Context, familyID string) (int64, error) {
	n, err := s.q(ctx).RevokeRefreshFamily(ctx, familyID)
	return n, database.MapErr(err)
}

func (s *Repository) RevokeRefreshBySessions(ctx context.Context, sessionIDs []string) (int64, error) {
	n, err := s.q(ctx).RevokeRefreshBySessions(ctx, sessionIDs)
	return n, database.MapErr(err)
}
