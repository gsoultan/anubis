package identitypg

import (
	"context"

	"github.com/gsoultan/anubis/internal/identity/adapter/postgres/rgen/realmcategory"
	identityrquery "github.com/gsoultan/anubis/internal/identity/adapter/postgres/rquery"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) ListRealms(ctx context.Context, tenantID string) ([]identitydomain.RealmRecord, error) {
	rows, err := identityrquery.ListRealms.Query(ctx, s.ex(ctx), tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]identitydomain.RealmRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, identitydomain.RealmRecord{
			ID: r.ID, Code: r.Code, Kind: r.Kind, DisplayName: r.DisplayName,
			MinAssurance: int(r.MinAssurance), SelfRegistration: r.SelfRegistration,
			EmailVerification: r.EmailVerificationRequired, PIIEncryption: r.PiiEncryption,
			AllowedFactors: r.AllowedFactors, RequiredFactors: r.RequiredFactors,
			SessionTTL: r.SessionTtl, AccessTokenTTL: r.AccessTokenTtl,
			RefreshTokenTTL: r.RefreshTokenTtl, DefaultRetention: r.DefaultRetention,
			PasswordPolicy:          []byte(r.PasswordPolicy),
			FactorEnrolmentDeadline: tptr(r.FactorEnrolmentDeadline),
		})
	}
	return out, nil
}

func (s *Repository) CreateRealm(ctx context.Context, tenantID string, r identitydomain.RealmRecord) (string, error) {
	row, _, err := identityrquery.CreateRealm.One(ctx, s.ex(ctx),
		tenantID, r.Code, r.Kind, r.DisplayName, int16(r.MinAssurance),
		r.SelfRegistration, r.EmailVerification, r.PIIEncryption,
		database.EmptyIfNil(r.AllowedFactors), database.EmptyIfNil(r.RequiredFactors),
		database.OrEmptyJSON(r.PasswordPolicy),
		database.OrDefaultStr(r.SessionTTL, "12 hours"),
		database.OrDefaultStr(r.AccessTokenTTL, "10 minutes"),
		database.OrDefaultStr(r.RefreshTokenTTL, "30 days"),
		r.DefaultRetention, r.FactorEnrolmentDeadline)
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.ID, nil
}

// UpdateRealm changes a realm's POLICY. code and kind are not part of it:
// they decide which roles a realm's members may hold, so changing them on a
// populated realm would retroactively re-decide access already granted.
func (s *Repository) UpdateRealm(ctx context.Context, tenantID string, r identitydomain.RealmRecord) error {
	_, ok, err := identityrquery.UpdateRealm.One(ctx, s.ex(ctx),
		r.ID, r.DisplayName, int16(r.MinAssurance), r.SelfRegistration,
		r.EmailVerification, r.PIIEncryption,
		database.EmptyIfNil(r.AllowedFactors), database.EmptyIfNil(r.RequiredFactors),
		database.OrEmptyJSON(r.PasswordPolicy),
		database.OrDefaultStr(r.SessionTTL, "12 hours"),
		database.OrDefaultStr(r.AccessTokenTTL, "10 minutes"),
		database.OrDefaultStr(r.RefreshTokenTTL, "30 days"),
		r.DefaultRetention, r.FactorEnrolmentDeadline, tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	// No row: either the realm does not exist or it belongs to another
	// tenant. The two are one answer on purpose — telling them apart would
	// confirm a realm id to a tenant that cannot see it.
	if !ok {
		return apperr.ErrNotFound
	}
	return nil
}

// CountIdentitiesByCategory counts people per category in one grouped query —
// the Populations screen's figures, computed where the rows are. The console
// used to count rows it had fetched, capped at 2,000 of 57,000.
func (s *Repository) CountIdentitiesByCategory(ctx context.Context, tenantID, realmID string) (map[string]int64, error) {
	// An empty realm is "every realm", and handing PostgreSQL '' for a uuid is
	// an error rather than an empty filter — which is how a tenant-wide
	// listing used to 500.
	rows, err := identityrquery.CountIdentitiesByCategory.Query(ctx, s.ex(ctx),
		tenantID, optArg(realmID))
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.CategoryID] = r.N
	}
	return out, nil
}

// ListRealmCategories is tenant-scoped ALWAYS, realm optional.
//
// The tenant filter is not decoration. This used to key on realm_id alone, so
// an operator who learned another tenant's realm id read that tenant's
// categories: the caller's tenant was checked by the guard and then never
// used.
func (s *Repository) ListRealmCategories(ctx context.Context, tenantID, realmID string) ([]identitydomain.RealmCategoryRecord, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	q := realmcategory.New().
		Where(realmcategory.TenantID.Eq(tid)).
		Order(realmcategory.RealmID.Asc(), realmcategory.SortOrder.Asc(), realmcategory.Code.Asc())
	if realmID != "" {
		rid, err := database.ParseUUID(realmID)
		if err != nil {
			return nil, database.MapErr(err)
		}
		q = q.Where(realmcategory.RealmID.Eq(rid))
	}
	rows, err := q.All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]identitydomain.RealmCategoryRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, categoryRecord(r))
	}
	return out, nil
}

func categoryRecord(r realmcategory.Row) identitydomain.RealmCategoryRecord {
	return identitydomain.RealmCategoryRecord{
		ID: database.UUIDStr(r.ID), RealmID: database.UUIDStr(r.RealmID),
		Code: r.Code, DisplayName: r.DisplayName, SortOrder: int(r.SortOrder),
	}
}

func (s *Repository) CreateRealmCategory(ctx context.Context, tenantID string, c identitydomain.RealmCategoryRecord) (string, error) {
	tid, rid, err := twoUUIDs(tenantID, c.RealmID)
	if err != nil {
		return "", err
	}
	n := realmcategory.Create()
	n.SetTenantID(tid)
	n.SetRealmID(rid)
	n.SetCode(c.Code)
	n.SetDisplayName(c.DisplayName)
	n.SetSortOrder(int32(c.SortOrder))
	row, err := n.Insert(ctx, s.ex(ctx))
	if err != nil {
		return "", database.MapErr(err)
	}
	return database.UUIDStr(row.ID), nil
}

func (s *Repository) RealmCategoryByCode(ctx context.Context, realmID, code string) (*identitydomain.RealmCategoryRecord, error) {
	rid, err := database.ParseUUID(realmID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	row, ok, err := realmcategory.New().
		Where(realmcategory.RealmID.Eq(rid), realmcategory.Code.Eq(code)).
		One(ctx, s.ex(ctx))
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	rec := categoryRecord(row)
	return &rec, nil
}

// CorrectEmptyRealmIdentity fixes a typo in a realm's code or kind, and only
// while the realm is empty.
func (s *Repository) CorrectEmptyRealmIdentity(ctx context.Context, tenantID, realmID, code, kind string) (bool, error) {
	n, err := identityrquery.CorrectEmptyRealmIdentity.Exec(ctx, s.ex(ctx),
		realmID, tenantID, code, kind)
	if err != nil {
		return false, database.MapErr(err)
	}
	// Zero rows means the realm has members — the statement's own NOT EXISTS
	// refused it, so there is no window between deciding and writing.
	return n > 0, nil
}
