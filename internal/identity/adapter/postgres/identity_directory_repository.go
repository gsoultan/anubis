package identitypg

import (
	"context"

	identityrquery "github.com/gsoultan/anubis/internal/identity/adapter/postgres/rquery"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// ListIdentities is one KEYSET page of a tenant's people.
//
// The cursor is the last id, not an offset: uuidv7 is time-ordered, so id
// order IS creation order and the probe stays an index range scan at any
// depth — which is what makes the last page of a 57,000-row tenant cost the
// same as the first.
func (s *Repository) ListIdentities(ctx context.Context, tenantID string, f identitydomain.IdentityFilter) ([]identitydomain.IdentityRecord, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := identityrquery.ListIdentities.Query(ctx, s.ex(ctx),
		tenantID, optArg(f.RealmID), optArg(f.Status), optArg(f.Query),
		optArg(f.AfterID), int32(limit))
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]identitydomain.IdentityRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, recordFromRow(r))
	}
	return out, nil
}

func (s *Repository) IdentityRecordByID(ctx context.Context, tenantID, id string) (*identitydomain.IdentityRecord, error) {
	r, ok, err := identityrquery.GetIdentity.One(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	rec := recordFromRow(r)
	return &rec, nil
}

func recordFromRow(r identityrquery.IdentityRow) identitydomain.IdentityRecord {
	return identityRecordFromRow(
		r.ID, r.Username, nstr(r.Email), nstr(r.RealmCode), nstr(r.RealmKind),
		r.Status, nstr(r.CategoryCode), nstr(r.ExternalRef),
		int(r.AssuranceLevel), int(r.TokenEpoch), r.CreatedAt,
		tptr(r.LastLoginAt), tptr(r.DisabledAt), tptr(r.AnonymizedAt),
		tptr(r.RetentionUntil))
}
