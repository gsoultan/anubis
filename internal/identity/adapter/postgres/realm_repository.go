package identitypg

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/identity/adapter/postgres/gen"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) RealmByCode(ctx context.Context, tenantID, code string) (*identitydomain.Realm, error) {
	row, err := s.q(ctx).GetRealmByCode(ctx, gen.GetRealmByCodeParams{TenantID: tenantID, Code: code})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return realmFromParts(row.ID, row.TenantID, row.Code, row.Kind, row.DisplayName,
		int(row.MinAssurance), row.SelfRegistration, row.EmailVerificationRequired,
		row.AllowedFactors, row.RequiredFactors, row.PasswordPolicy,
		row.SessionTtlSecs, row.AccessTokenTtlSecs, row.RefreshTokenTtlSecs,
		row.FactorEnrolmentDeadline), nil
}

func (s *Repository) RealmByID(ctx context.Context, id string) (*identitydomain.Realm, error) {
	row, err := s.q(ctx).GetRealm(ctx, id)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return realmFromParts(row.ID, row.TenantID, row.Code, row.Kind, row.DisplayName,
		int(row.MinAssurance), row.SelfRegistration, row.EmailVerificationRequired,
		row.AllowedFactors, row.RequiredFactors, row.PasswordPolicy,
		row.SessionTtlSecs, row.AccessTokenTtlSecs, row.RefreshTokenTtlSecs,
		row.FactorEnrolmentDeadline), nil
}

func realmFromParts(id, tenantID, code, kind, name string, minAssurance int,
	selfReg, emailVerify bool, allowed, required []string, policy []byte,
	sessionSecs, accessSecs, refreshSecs int64,
	enrolmentDeadline *time.Time) *identitydomain.Realm {
	return &identitydomain.Realm{
		ID: id, TenantID: tenantID, Code: code, Kind: kind, DisplayName: name,
		MinAssurance: minAssurance, SelfRegistration: selfReg,
		EmailVerification: emailVerify,
		AllowedFactors:    allowed, RequiredFactors: required,
		SessionTTL:      time.Duration(sessionSecs) * time.Second,
		AccessTokenTTL:  time.Duration(accessSecs) * time.Second,
		RefreshTokenTTL: time.Duration(refreshSecs) * time.Second,
		PasswordPolicy:  identitydomain.ParsePasswordPolicy(policy),
		// Nil means the realm has not started enforcing enrolment, which is
		// the zero value the domain reads as "not in force".
		FactorEnrolmentDeadline: derefTime(enrolmentDeadline),
	}
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
