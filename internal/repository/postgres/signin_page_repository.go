package postgres

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
)

func (s *Store) SigninPage(ctx context.Context, tenantID string) ([]byte, time.Time, error) {
	row, err := s.q(ctx).GetSigninPage(ctx, tenantID)
	if err != nil {
		return nil, time.Time{}, mapErr(err)
	}
	return row.Config, row.UpdatedAt, nil
}

func (s *Store) PutSigninPage(ctx context.Context, tenantID string, config []byte) error {
	return mapErr(s.q(ctx).PutSigninPage(ctx, gen.PutSigninPageParams{
		TenantID: tenantID, Config: orEmptyJSON(config),
	}))
}
