package postgres

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) CreateRefresh(ctx context.Context, in repository.RefreshInput) (string, error) {
	id, err := s.q(ctx).CreateRefreshToken(ctx, gen.CreateRefreshTokenParams{
		SessionID: in.SessionID, TenantID: in.TenantID, FamilyID: in.FamilyID,
		Generation: int32(in.Generation), TokenHash: in.TokenHash,
		ExpiresAt: in.ExpiresAt, BoundKey: in.BoundKey,
	})
	return id, mapErr(err)
}

func (s *Store) ClaimRefresh(ctx context.Context, hash []byte) (*repository.RefreshClaim, error) {
	row, err := s.q(ctx).ClaimRefreshToken(ctx, hash)
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.RefreshClaim{
		ID: row.ID, SessionID: row.SessionID, TenantID: row.TenantID,
		FamilyID: row.FamilyID, Generation: int(row.Generation), ExpiresAt: row.ExpiresAt,
	}, nil
}

func (s *Store) RefreshByHash(ctx context.Context, hash []byte) (*repository.RefreshInfo, error) {
	row, err := s.q(ctx).GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.RefreshInfo{
		ID: row.ID, SessionID: row.SessionID, TenantID: row.TenantID,
		FamilyID: row.FamilyID, Status: row.Status, ExpiresAt: row.ExpiresAt,
	}, nil
}

func (s *Store) SetRefreshSuccessor(ctx context.Context, id string, expiresAt time.Time, successorID string) error {
	return mapErr(s.q(ctx).SetRefreshSuccessor(ctx, gen.SetRefreshSuccessorParams{
		ID: id, ExpiresAt: expiresAt, SuccessorID: optStr(successorID),
	}))
}

func (s *Store) RevokeRefreshFamily(ctx context.Context, familyID string) (int64, error) {
	n, err := s.q(ctx).RevokeRefreshFamily(ctx, familyID)
	return n, mapErr(err)
}

func (s *Store) RevokeRefreshBySessions(ctx context.Context, sessionIDs []string) (int64, error) {
	n, err := s.q(ctx).RevokeRefreshBySessions(ctx, sessionIDs)
	return n, mapErr(err)
}
