package gatepg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
)

// Tenants implements gateapp.TenantLister.
func (s *Repository) Tenants(ctx context.Context) ([]string, []string, error) {
	rows, err := s.q(ctx).SnapshotTenants(ctx)
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
