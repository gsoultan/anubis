package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) ListIdentities(ctx context.Context, tenantID string, f repository.IdentityFilter) ([]repository.IdentityRecord, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.q(ctx).ListIdentities(ctx, gen.ListIdentitiesParams{
		TenantID: tenantID,
		RealmID:  optStr(f.RealmID),
		Status:   optStr(f.Status),
		Query:    optStr(f.Query),
		AfterID:  optStr(f.AfterID),
		PageSize: int32(limit),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.IdentityRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, identityRecordFromRow(
			r.ID, r.Username, deref(r.Email), deref(r.RealmCode), deref(r.RealmKind),
			r.Status, deref(r.CategoryCode), deref(r.ExternalRef),
			int(r.AssuranceLevel), int(r.TokenEpoch), r.CreatedAt,
			r.LastLoginAt, r.DisabledAt, r.AnonymizedAt))
	}
	return out, nil
}

func (s *Store) IdentityRecordByID(ctx context.Context, tenantID, id string) (*repository.IdentityRecord, error) {
	r, err := s.q(ctx).GetIdentity(ctx, gen.GetIdentityParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, mapErr(err)
	}
	rec := identityRecordFromRow(
		r.ID, r.Username, deref(r.Email), deref(r.RealmCode), deref(r.RealmKind),
		r.Status, deref(r.CategoryCode), deref(r.ExternalRef),
		int(r.AssuranceLevel), int(r.TokenEpoch), r.CreatedAt,
		r.LastLoginAt, r.DisabledAt, r.AnonymizedAt)
	return &rec, nil
}
