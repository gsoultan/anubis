package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) ListRealms(ctx context.Context, tenantID string) ([]repository.RealmRecord, error) {
	rows, err := s.q(ctx).ListRealms(ctx, tenantID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.RealmRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.RealmRecord{
			ID: r.ID, Code: r.Code, Kind: r.Kind, DisplayName: r.DisplayName,
			MinAssurance: int(r.MinAssurance), SelfRegistration: r.SelfRegistration,
			EmailVerification: r.EmailVerificationRequired, PIIEncryption: r.PiiEncryption,
			AllowedFactors: r.AllowedFactors, RequiredFactors: r.RequiredFactors,
			SessionTTL: r.SessionTtl, AccessTokenTTL: r.AccessTokenTtl,
			RefreshTokenTTL: r.RefreshTokenTtl, DefaultRetention: r.DefaultRetention,
			PasswordPolicy: r.PasswordPolicy,
		})
	}
	return out, nil
}

func (s *Store) CreateRealm(ctx context.Context, tenantID string, r repository.RealmRecord) (string, error) {
	id, err := s.q(ctx).CreateRealm(ctx, gen.CreateRealmParams{
		TenantID: tenantID, Code: r.Code, Kind: r.Kind, DisplayName: r.DisplayName,
		MinAssurance: int16(r.MinAssurance), SelfRegistration: r.SelfRegistration,
		EmailVerificationRequired: r.EmailVerification, PiiEncryption: r.PIIEncryption,
		AllowedFactors: emptyIfNil(r.AllowedFactors), RequiredFactors: emptyIfNil(r.RequiredFactors),
		PasswordPolicy:   orEmptyJSON(r.PasswordPolicy),
		SessionTtl:       orDefaultStr(r.SessionTTL, "12 hours"),
		AccessTokenTtl:   orDefaultStr(r.AccessTokenTTL, "10 minutes"),
		RefreshTokenTtl:  orDefaultStr(r.RefreshTokenTTL, "30 days"),
		DefaultRetention: r.DefaultRetention,
	})
	return id, mapErr(err)
}

func (s *Store) UpdateRealm(ctx context.Context, tenantID string, r repository.RealmRecord) error {
	_ = tenantID // realms are updated by their own id; tenancy is checked by the caller loading the record
	_, err := s.q(ctx).UpdateRealm(ctx, gen.UpdateRealmParams{
		ID: r.ID, DisplayName: r.DisplayName, MinAssurance: int16(r.MinAssurance),
		SelfRegistration: r.SelfRegistration, EmailVerificationRequired: r.EmailVerification,
		PiiEncryption:  r.PIIEncryption,
		AllowedFactors: emptyIfNil(r.AllowedFactors), RequiredFactors: emptyIfNil(r.RequiredFactors),
		PasswordPolicy:   orEmptyJSON(r.PasswordPolicy),
		SessionTtl:       orDefaultStr(r.SessionTTL, "12 hours"),
		AccessTokenTtl:   orDefaultStr(r.AccessTokenTTL, "10 minutes"),
		RefreshTokenTtl:  orDefaultStr(r.RefreshTokenTTL, "30 days"),
		DefaultRetention: r.DefaultRetention,
	})
	return mapErr(err)
}

func (s *Store) ListRealmCategories(ctx context.Context, realmID string) ([]repository.RealmCategoryRecord, error) {
	rows, err := s.q(ctx).ListRealmCategories(ctx, realmID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.RealmCategoryRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.RealmCategoryRecord{
			ID: r.ID, RealmID: r.RealmID, Code: r.Code,
			DisplayName: r.DisplayName, SortOrder: int(r.SortOrder),
		})
	}
	return out, nil
}

func (s *Store) CreateRealmCategory(ctx context.Context, tenantID string, c repository.RealmCategoryRecord) (string, error) {
	row, err := s.q(ctx).CreateRealmCategory(ctx, gen.CreateRealmCategoryParams{
		TenantID: tenantID, RealmID: c.RealmID, Code: c.Code,
		DisplayName: c.DisplayName, SortOrder: int32(c.SortOrder),
	})
	if err != nil {
		return "", mapErr(err)
	}
	return row.ID, nil
}

func (s *Store) RealmCategoryByCode(ctx context.Context, realmID, code string) (*repository.RealmCategoryRecord, error) {
	r, err := s.q(ctx).GetRealmCategoryByCode(ctx, gen.GetRealmCategoryByCodeParams{
		RealmID: realmID, Code: code,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.RealmCategoryRecord{
		ID: r.ID, RealmID: r.RealmID, Code: r.Code,
		DisplayName: r.DisplayName, SortOrder: int(r.SortOrder),
	}, nil
}

func derefS(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func orDefaultStr(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
