package tenancypg

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/platform/database"
	tenancyrquery "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/rquery"
)

// SigninPage reads a tenant's default sign-in page through the signin_pages
// view, which is migrations/0018's shape kept working after 0024 moved pages
// into their own table.
func (s *Repository) SigninPage(ctx context.Context, tenantID string) ([]byte, time.Time, error) {
	row, _, err := tenancyrquery.GetSigninPage.One(ctx, s.ex(ctx), tenantID)
	if err != nil {
		return nil, time.Time{}, database.MapErr(err)
	}
	return []byte(row.Config), row.UpdatedAt, nil
}

func (s *Repository) PutSigninPage(ctx context.Context, tenantID string, config []byte) error {
	_, err := tenancyrquery.PutSigninPage.Exec(ctx, s.ex(ctx),
		tenantID, database.OrEmptyJSON(config))
	return database.MapErr(err)
}
