package identitypg

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/identity/adapter/postgres/gen"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) ListRealms(ctx context.Context, tenantID string) ([]identitydomain.RealmRecord, error) {
	rows, err := s.q(ctx).ListRealms(ctx, tenantID)
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
			PasswordPolicy: r.PasswordPolicy, FactorEnrolmentDeadline: r.FactorEnrolmentDeadline,
		})
	}
	return out, nil
}

func (s *Repository) CreateRealm(ctx context.Context, tenantID string, r identitydomain.RealmRecord) (string, error) {
	id, err := s.q(ctx).CreateRealm(ctx, gen.CreateRealmParams{
		TenantID: tenantID, Code: r.Code, Kind: r.Kind, DisplayName: r.DisplayName,
		MinAssurance: int16(r.MinAssurance), SelfRegistration: r.SelfRegistration,
		EmailVerificationRequired: r.EmailVerification, PiiEncryption: r.PIIEncryption,
		AllowedFactors: database.EmptyIfNil(r.AllowedFactors), RequiredFactors: database.EmptyIfNil(r.RequiredFactors),
		PasswordPolicy:          database.OrEmptyJSON(r.PasswordPolicy),
		SessionTtl:              database.OrDefaultStr(r.SessionTTL, "12 hours"),
		AccessTokenTtl:          database.OrDefaultStr(r.AccessTokenTTL, "10 minutes"),
		RefreshTokenTtl:         database.OrDefaultStr(r.RefreshTokenTTL, "30 days"),
		DefaultRetention:        r.DefaultRetention,
		FactorEnrolmentDeadline: r.FactorEnrolmentDeadline,
	})
	return id, database.MapErr(err)
}

func (s *Repository) UpdateRealm(ctx context.Context, tenantID string, r identitydomain.RealmRecord) error {
	_ = tenantID // realms are updated by their own id; tenancy is checked by the caller loading the record
	_, err := s.q(ctx).UpdateRealm(ctx, gen.UpdateRealmParams{
		ID: r.ID, DisplayName: r.DisplayName, MinAssurance: int16(r.MinAssurance),
		SelfRegistration: r.SelfRegistration, EmailVerificationRequired: r.EmailVerification,
		PiiEncryption:  r.PIIEncryption,
		AllowedFactors: database.EmptyIfNil(r.AllowedFactors), RequiredFactors: database.EmptyIfNil(r.RequiredFactors),
		PasswordPolicy:          database.OrEmptyJSON(r.PasswordPolicy),
		SessionTtl:              database.OrDefaultStr(r.SessionTTL, "12 hours"),
		AccessTokenTtl:          database.OrDefaultStr(r.AccessTokenTTL, "10 minutes"),
		RefreshTokenTtl:         database.OrDefaultStr(r.RefreshTokenTTL, "30 days"),
		DefaultRetention:        r.DefaultRetention,
		FactorEnrolmentDeadline: r.FactorEnrolmentDeadline,
	})
	return database.MapErr(err)
}

// CountIdentitiesByCategory counts people per category in one grouped
// query — the Populations screen's figures, computed where the rows are.
func (s *Repository) CountIdentitiesByCategory(ctx context.Context, tenantID, realmID string) (map[string]int64, error) {
	rows, err := s.q(ctx).CountIdentitiesByCategory(ctx, gen.CountIdentitiesByCategoryParams{
		TenantID: tenantID, RealmID: &realmID,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[database.Deref(r.CategoryID)] = r.N
	}
	return out, nil
}

func (s *Repository) ListRealmCategories(ctx context.Context, realmID string) ([]identitydomain.RealmCategoryRecord, error) {
	rows, err := s.q(ctx).ListRealmCategories(ctx, realmID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]identitydomain.RealmCategoryRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, identitydomain.RealmCategoryRecord{
			ID: r.ID, RealmID: r.RealmID, Code: r.Code,
			DisplayName: r.DisplayName, SortOrder: int(r.SortOrder),
		})
	}
	return out, nil
}

func (s *Repository) CreateRealmCategory(ctx context.Context, tenantID string, c identitydomain.RealmCategoryRecord) (string, error) {
	row, err := s.q(ctx).CreateRealmCategory(ctx, gen.CreateRealmCategoryParams{
		TenantID: tenantID, RealmID: c.RealmID, Code: c.Code,
		DisplayName: c.DisplayName, SortOrder: int32(c.SortOrder),
	})
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.ID, nil
}

func (s *Repository) RealmCategoryByCode(ctx context.Context, realmID, code string) (*identitydomain.RealmCategoryRecord, error) {
	r, err := s.q(ctx).GetRealmCategoryByCode(ctx, gen.GetRealmCategoryByCodeParams{
		RealmID: realmID, Code: code,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &identitydomain.RealmCategoryRecord{
		ID: r.ID, RealmID: r.RealmID, Code: r.Code,
		DisplayName: r.DisplayName, SortOrder: int(r.SortOrder),
	}, nil
}

func (s *Repository) CorrectEmptyRealmIdentity(ctx context.Context, tenantID, realmID, code, kind string) (bool, error) {
	n, err := s.q(ctx).CorrectEmptyRealmIdentity(ctx, gen.CorrectEmptyRealmIdentityParams{
		ID: realmID, TenantID: tenantID, Code: code, Kind: kind,
	})
	if err != nil {
		return false, database.MapErr(err)
	}
	// Zero rows means the realm has members — the statement's own NOT EXISTS
	// refused it, so there is no window between deciding and writing.
	return n > 0, nil
}
