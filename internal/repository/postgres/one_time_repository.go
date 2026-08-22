package postgres

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
)

func (s *Store) CreateOneTime(ctx context.Context, tenantID, kind string, hash []byte, payload []byte, expiresAt time.Time) (string, error) {
	id, err := s.q(ctx).CreateOneTimeToken(ctx, gen.CreateOneTimeTokenParams{
		TenantID: tenantID, Kind: kind, TokenHash: hash,
		Payload: orEmptyJSON(payload), ExpiresAt: expiresAt,
	})
	return id, mapErr(err)
}

func (s *Store) ConsumeOneTime(ctx context.Context, kind string, hash []byte) (string, []byte, error) {
	row, err := s.q(ctx).ConsumeOneTimeToken(ctx, gen.ConsumeOneTimeTokenParams{
		TokenHash: hash, Kind: kind,
	})
	if err != nil {
		return "", nil, mapErr(err)
	}
	return row.TenantID, row.Payload, nil
}
