package identitypg

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/identity/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) ApplyRealmRetention(ctx context.Context) (int64, error) {
	n, err := s.q(ctx).SetRetentionFromRealm(ctx)
	return n, database.MapErr(err)
}

func (s *Repository) ExpireRetained(ctx context.Context) ([]string, []string, []string, error) {
	rows, err := s.q(ctx).ExpireRetainedIdentities(ctx)
	if err != nil {
		return nil, nil, nil, database.MapErr(err)
	}
	ids := make([]string, 0, len(rows))
	tenants := make([]string, 0, len(rows))
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
		tenants = append(tenants, r.TenantID)
		if r.PiiKeyID != nil {
			keys = append(keys, *r.PiiKeyID)
		}
	}
	return ids, tenants, keys, nil
}

func (s *Repository) Anonymize(ctx context.Context, tenantID, identityID string) (string, error) {
	row, err := s.q(ctx).AnonymizeIdentity(ctx, gen.AnonymizeIdentityParams{
		ID: identityID, TenantID: tenantID,
	})
	if err != nil {
		return "", database.MapErr(err)
	}
	return database.Deref(row.PiiKeyID), nil
}
