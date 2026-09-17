package identitypg

import (
	"context"
	"time"

	identityrquery "github.com/gsoultan/anubis/internal/identity/adapter/postgres/rquery"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) RealmByCode(ctx context.Context, tenantID, code string) (*identitydomain.Realm, error) {
	row, ok, err := identityrquery.GetRealmByCode.One(ctx, s.ex(ctx), tenantID, code)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	return realmFromRow(row), nil
}

func (s *Repository) RealmByID(ctx context.Context, id string) (*identitydomain.Realm, error) {
	row, ok, err := identityrquery.GetRealm.One(ctx, s.ex(ctx), id)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	return realmFromRow(row), nil
}

// realmFromRow maps the policy a realm governs its population with.
//
// The TTLs arrive as seconds and become durations here; the text forms are for
// the console, which edits them.
func realmFromRow(r identityrquery.RealmWithSecsRow) *identitydomain.Realm {
	return &identitydomain.Realm{
		ID: r.ID, TenantID: r.TenantID, Code: r.Code, Kind: r.Kind,
		DisplayName:       r.DisplayName,
		MinAssurance:      int(r.MinAssurance),
		SelfRegistration:  r.SelfRegistration,
		EmailVerification: r.EmailVerificationRequired,
		AllowedFactors:    r.AllowedFactors,
		RequiredFactors:   r.RequiredFactors,
		SessionTTL:        time.Duration(r.SessionTtlSecs) * time.Second,
		AccessTokenTTL:    time.Duration(r.AccessTokenTtlSecs) * time.Second,
		RefreshTokenTTL:   time.Duration(r.RefreshTokenTtlSecs) * time.Second,
		PasswordPolicy:    identitydomain.ParsePasswordPolicy([]byte(r.PasswordPolicy)),
		// The zero time means the realm has not started enforcing enrolment,
		// which is what the domain reads as "not in force".
		FactorEnrolmentDeadline: zeroTime(tptr(r.FactorEnrolmentDeadline)),
	}
}

func zeroTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
