package authpg

import (
	"context"
	"time"

	authrquery "github.com/gsoultan/anubis/internal/auth/adapter/postgres/rquery"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) CreateRefresh(ctx context.Context, in authdomain.RefreshInput) (string, error) {
	row, _, err := authrquery.CreateRefreshToken.One(ctx, s.ex(ctx),
		in.SessionID, in.TenantID, in.FamilyID, in.Generation,
		in.TokenHash, in.ExpiresAt, in.BoundKey)
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.ID, nil
}

// ClaimRefresh is the rotation core: exactly one caller can flip
// active -> consumed. nil means the token was not claimable, and the caller
// then reads it by hash — a consumed one is what theft looks like.
func (s *Repository) ClaimRefresh(ctx context.Context, hash []byte) (*authdomain.RefreshClaim, error) {
	row, ok, err := authrquery.ClaimRefreshToken.One(ctx, s.ex(ctx), hash)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, nil
	}
	return &authdomain.RefreshClaim{
		ID: row.ID, SessionID: row.SessionID, TenantID: row.TenantID,
		FamilyID: row.FamilyID, Generation: int(row.Generation),
		ExpiresAt: row.ExpiresAt,
	}, nil
}

// RefreshByHash reads a token in WHATEVER state, which is the point: a
// consumed or revoked one presented again is theft, not a miss.
func (s *Repository) RefreshByHash(ctx context.Context, hash []byte) (*authdomain.RefreshInfo, error) {
	row, ok, err := authrquery.GetRefreshTokenByHash.One(ctx, s.ex(ctx), hash)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, nil
	}
	return &authdomain.RefreshInfo{
		ID: row.ID, SessionID: row.SessionID, TenantID: row.TenantID,
		FamilyID: row.FamilyID, Status: row.Status, ExpiresAt: row.ExpiresAt,
	}, nil
}

// SetRefreshSuccessor links a consumed token to the one that replaced it.
// expires_at is passed because the table is partitioned on it: without it the
// UPDATE would scan every partition to find one row.
func (s *Repository) SetRefreshSuccessor(ctx context.Context, id string, expiresAt time.Time, successorID string) error {
	_, err := authrquery.SetRefreshSuccessor.Exec(ctx, s.ex(ctx), id, successorID, expiresAt)
	return database.MapErr(err)
}

// RevokeRefreshFamily is the theft response: the whole family dies, whatever
// generation the attacker holds.
func (s *Repository) RevokeRefreshFamily(ctx context.Context, familyID string) (int64, error) {
	n, err := authrquery.RevokeRefreshFamily.Exec(ctx, s.ex(ctx), familyID)
	return n, database.MapErr(err)
}

func (s *Repository) RevokeRefreshBySession(ctx context.Context, sessionID string) (int64, error) {
	n, err := authrquery.RevokeRefreshBySession.Exec(ctx, s.ex(ctx), sessionID)
	return n, database.MapErr(err)
}

// RevokeRefreshBySessions takes a list, so signing out everywhere is one
// statement rather than one per session.
func (s *Repository) RevokeRefreshBySessions(ctx context.Context, sessionIDs []string) (int64, error) {
	if len(sessionIDs) == 0 {
		return 0, nil
	}
	n, err := authrquery.RevokeRefreshBySessions.Exec(ctx, s.ex(ctx), sessionIDs)
	return n, database.MapErr(err)
}
