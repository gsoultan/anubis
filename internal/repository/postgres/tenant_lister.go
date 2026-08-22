package postgres

import "context"

// Tenants implements snapshot.TenantLister.
func (s *Store) Tenants(ctx context.Context) ([]string, []string, error) {
	rows, err := s.q(ctx).SnapshotTenants(ctx)
	if err != nil {
		return nil, nil, mapErr(err)
	}
	ids := make([]string, 0, len(rows))
	slugs := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
		slugs = append(slugs, r.Slug)
	}
	return ids, slugs, nil
}
