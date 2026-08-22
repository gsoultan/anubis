package tenancypg

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/platform/database"
	gen "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/gen"
)

func (s *Repository) SigninPage(ctx context.Context, tenantID string) ([]byte, time.Time, error) {
	row, err := s.q(ctx).GetSigninPage(ctx, tenantID)
	if err != nil {
		return nil, time.Time{}, database.MapErr(err)
	}
	return row.Config, row.UpdatedAt, nil
}

func (s *Repository) PutSigninPage(ctx context.Context, tenantID string, config []byte) error {
	return database.MapErr(s.q(ctx).PutSigninPage(ctx, gen.PutSigninPageParams{
		TenantID: tenantID, Config: database.OrEmptyJSON(config),
	}))
}
