package authpg

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/auth/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) CreateOneTime(ctx context.Context, tenantID, kind string, hash []byte, payload []byte, expiresAt time.Time) (string, error) {
	id, err := s.q(ctx).CreateOneTimeToken(ctx, gen.CreateOneTimeTokenParams{
		TenantID: tenantID, Kind: kind, TokenHash: hash,
		Payload: database.OrEmptyJSON(payload), ExpiresAt: expiresAt,
	})
	return id, database.MapErr(err)
}

func (s *Repository) ConsumeOneTime(ctx context.Context, kind string, hash []byte) (string, []byte, error) {
	row, err := s.q(ctx).ConsumeOneTimeToken(ctx, gen.ConsumeOneTimeTokenParams{
		TokenHash: hash, Kind: kind,
	})
	if err != nil {
		return "", nil, database.MapErr(err)
	}
	return row.TenantID, row.Payload, nil
}
