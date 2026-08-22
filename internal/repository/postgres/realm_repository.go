package postgres

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/domain"
)

func (s *Store) RealmByCode(ctx context.Context, tenantID, code string) (*domain.Realm, error) {
	row, err := s.q(ctx).GetRealmByCode(ctx, gen.GetRealmByCodeParams{TenantID: tenantID, Code: code})
	if err != nil {
		return nil, mapErr(err)
	}
	return realmFromParts(row.ID, row.TenantID, row.Code, row.Kind, row.DisplayName,
		int(row.MinAssurance), row.SelfRegistration, row.EmailVerificationRequired,
		row.AllowedFactors, row.RequiredFactors, row.PasswordPolicy,
		row.SessionTtlSecs, row.AccessTokenTtlSecs, row.RefreshTokenTtlSecs), nil
}

func (s *Store) RealmByID(ctx context.Context, id string) (*domain.Realm, error) {
	row, err := s.q(ctx).GetRealm(ctx, id)
	if err != nil {
		return nil, mapErr(err)
	}
	return realmFromParts(row.ID, row.TenantID, row.Code, row.Kind, row.DisplayName,
		int(row.MinAssurance), row.SelfRegistration, row.EmailVerificationRequired,
		row.AllowedFactors, row.RequiredFactors, row.PasswordPolicy,
		row.SessionTtlSecs, row.AccessTokenTtlSecs, row.RefreshTokenTtlSecs), nil
}

func realmFromParts(id, tenantID, code, kind, name string, minAssurance int,
	selfReg, emailVerify bool, allowed, required []string, policy []byte,
	sessionSecs, accessSecs, refreshSecs int64) *domain.Realm {
	return &domain.Realm{
		ID: id, TenantID: tenantID, Code: code, Kind: kind, DisplayName: name,
		MinAssurance: minAssurance, SelfRegistration: selfReg,
		EmailVerification: emailVerify,
		AllowedFactors:    allowed, RequiredFactors: required,
		SessionTTL:      time.Duration(sessionSecs) * time.Second,
		AccessTokenTTL:  time.Duration(accessSecs) * time.Second,
		RefreshTokenTTL: time.Duration(refreshSecs) * time.Second,
		PasswordPolicy:  domain.ParsePasswordPolicy(policy),
	}
}
