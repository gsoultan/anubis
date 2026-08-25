package controlpg

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gsoultan/anubis/internal/control/adapter/postgres/gen"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// CreateRefreshFamily begins a sign-in; the row's own id is the family id,
// which is what lets one revocation kill the session however many times it
// rotated. Only the hash arrives here — the secret never touches this layer.
func (s *Repository) CreateRefreshFamily(ctx context.Context, platformUserID string, tokenHash []byte, expiresAt time.Time) (string, error) {
	id, err := s.q(ctx).InsertPlatformRefreshRoot(ctx, gen.InsertPlatformRefreshRootParams{
		PlatformUserID: platformUserID,
		TokenHash:      tokenHash,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		return "", database.MapErr(err)
	}
	return id, nil
}

// AppendRefresh adds a rotation's successor to an existing family.
func (s *Repository) AppendRefresh(ctx context.Context, platformUserID, familyID string, tokenHash []byte, expiresAt time.Time) error {
	_, err := s.q(ctx).InsertPlatformRefreshChild(ctx, gen.InsertPlatformRefreshChildParams{
		PlatformUserID: platformUserID,
		FamilyID:       familyID,
		TokenHash:      tokenHash,
		ExpiresAt:      expiresAt,
	})
	return database.MapErr(err)
}

func (s *Repository) RefreshByHash(ctx context.Context, tokenHash []byte) (*controldomain.PlatformRefresh, error) {
	row, err := s.q(ctx).PlatformRefreshByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, database.MapErr(err)
	}
	return &controldomain.PlatformRefresh{
		ID:             row.ID,
		PlatformUserID: row.PlatformUserID,
		FamilyID:       row.FamilyID,
		CreatedAt:      row.CreatedAt,
		ExpiresAt:      row.ExpiresAt,
		UsedAt:         row.UsedAt,
		RevokedAt:      row.RevokedAt,
	}, nil
}

// ConsumeRefresh is the rotation's atomic step: zero rows means somebody
// else spent this token first, and the caller must treat that as reuse.
func (s *Repository) ConsumeRefresh(ctx context.Context, id string) (bool, error) {
	n, err := s.q(ctx).ConsumePlatformRefresh(ctx, id)
	if err != nil {
		return false, database.MapErr(err)
	}
	return n == 1, nil
}

func (s *Repository) RevokeRefreshFamily(ctx context.Context, familyID string) error {
	_, err := s.q(ctx).RevokePlatformRefreshFamily(ctx, familyID)
	return database.MapErr(err)
}

func (s *Repository) SweepExpired(ctx context.Context) (int64, error) {
	n, err := s.q(ctx).SweepPlatformRefresh(ctx)
	if err != nil {
		return 0, database.MapErr(err)
	}
	return n, nil
}
