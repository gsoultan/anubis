package identitypg

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/identity/adapter/postgres/gen"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) ListIdentities(ctx context.Context, tenantID string, f identitydomain.IdentityFilter) ([]identitydomain.IdentityRecord, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.q(ctx).ListIdentities(ctx, gen.ListIdentitiesParams{
		TenantID: tenantID,
		RealmID:  database.OptStr(f.RealmID),
		Status:   database.OptStr(f.Status),
		Query:    database.OptStr(f.Query),
		AfterID:  database.OptStr(f.AfterID),
		PageSize: int32(limit),
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]identitydomain.IdentityRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, identityRecordFromRow(
			r.ID, r.Username, database.Deref(r.Email), database.Deref(r.RealmCode), database.Deref(r.RealmKind),
			r.Status, database.Deref(r.CategoryCode), database.Deref(r.ExternalRef),
			int(r.AssuranceLevel), int(r.TokenEpoch), r.CreatedAt,
			r.LastLoginAt, r.DisabledAt, r.AnonymizedAt))
	}
	return out, nil
}

func (s *Repository) IdentityRecordByID(ctx context.Context, tenantID, id string) (*identitydomain.IdentityRecord, error) {
	r, err := s.q(ctx).GetIdentity(ctx, gen.GetIdentityParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, database.MapErr(err)
	}
	rec := identityRecordFromRow(
		r.ID, r.Username, database.Deref(r.Email), database.Deref(r.RealmCode), database.Deref(r.RealmKind),
		r.Status, database.Deref(r.CategoryCode), database.Deref(r.ExternalRef),
		int(r.AssuranceLevel), int(r.TokenEpoch), r.CreatedAt,
		r.LastLoginAt, r.DisabledAt, r.AnonymizedAt)
	return &rec, nil
}
