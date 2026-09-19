package gatepg

import (
	"context"

	gaterquery "github.com/gsoultan/anubis/internal/gate/adapter/postgres/rquery"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// Tenants implements gateapp.TenantLister: every active tenant, which is what
// the manager builds a snapshot per.
func (s *Repository) Tenants(ctx context.Context) ([]string, []string, error) {
	rows, err := gaterquery.SnapshotTenants.Query(ctx, s.ex(ctx))
	if err != nil {
		return nil, nil, database.MapErr(err)
	}
	ids := make([]string, 0, len(rows))
	slugs := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
		slugs = append(slugs, r.Slug)
	}
	return ids, slugs, nil
}
