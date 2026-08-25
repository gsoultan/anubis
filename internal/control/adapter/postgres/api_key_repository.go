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

func (s *Repository) CreatePlatformAPIKey(ctx context.Context, ownerID, label, lookup, secretHash, createdBy string, expiresAt time.Time) (string, error) {
	arg := gen.CreatePlatformAPIKeyParams{
		PlatformUserID: ownerID, Label: label, Lookup: lookup,
		SecretHash: secretHash, ExpiresAt: expiresAt,
	}
	if createdBy != "" {
		arg.CreatedBy = &createdBy
	}
	row, err := s.q(ctx).CreatePlatformAPIKey(ctx, arg)
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.ID, nil
}

func (s *Repository) PlatformAPIKeyByLookup(ctx context.Context, lookup string) (*controldomain.PlatformAPIKeyAuth, error) {
	row, err := s.q(ctx).PlatformAPIKeyByLookup(ctx, lookup)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, database.MapErr(err)
	}
	return &controldomain.PlatformAPIKeyAuth{
		ID: row.ID, PlatformUserID: row.PlatformUserID, Username: row.Username,
		SecretHash: row.SecretHash, ExpiresAt: row.ExpiresAt,
		OwnerStatus: row.OwnerStatus, TokenEpoch: int(row.TokenEpoch),
	}, nil
}

// TouchPlatformAPIKeyUsed is best effort: failing to note the time is not a
// reason to refuse a caller entry.
func (s *Repository) TouchPlatformAPIKeyUsed(ctx context.Context, id string) {
	_ = s.q(ctx).TouchPlatformAPIKeyUsed(ctx, id)
}

func (s *Repository) ListPlatformAPIKeys(ctx context.Context) ([]controldomain.PlatformAPIKey, error) {
	rows, err := s.q(ctx).ListPlatformAPIKeys(ctx)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]controldomain.PlatformAPIKey, 0, len(rows))
	for _, r := range rows {
		out = append(out, controldomain.PlatformAPIKey{
			ID: r.ID, PlatformUserID: r.PlatformUserID, Username: r.Username,
			Label: r.Label, Lookup: r.Lookup, CreatedAt: r.CreatedAt,
			LastUsedAt: r.LastUsedAt, ExpiresAt: r.ExpiresAt, RevokedAt: r.RevokedAt,
		})
	}
	return out, nil
}

func (s *Repository) RevokePlatformAPIKey(ctx context.Context, id string) error {
	_, err := s.q(ctx).RevokePlatformAPIKey(ctx, id)
	return database.MapErr(err)
}
