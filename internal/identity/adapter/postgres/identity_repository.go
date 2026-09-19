package identitypg

import (
	"context"

	identityrquery "github.com/gsoultan/anubis/internal/identity/adapter/postgres/rquery"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// IdentityForLogin is the sign-in lookup. It matches on lower(username) —
// the expression the unique index is built on — and carries the realm's policy
// along so the hot path needs no second round trip.
func (s *Repository) IdentityForLogin(ctx context.Context, tenantID, realmID, username string) (*identitydomain.Identity, error) {
	row, ok, err := identityrquery.GetIdentityForLogin.One(ctx, s.ex(ctx),
		tenantID, optArg(realmID), username)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	return &identitydomain.Identity{
		ID:             row.ID,
		TenantID:       row.TenantID,
		RealmID:        nstr(row.RealmID),
		RealmCode:      nstr(row.RealmCode),
		RealmKind:      nstr(row.RealmKind),
		Username:       row.Username,
		Email:          nstr(row.Email),
		Status:         row.Status,
		AssuranceLevel: int(row.AssuranceLevel),
		TokenEpoch:     int(row.TokenEpoch),
		Disabled:       present(row.DisabledAt),
		Anonymized:     present(row.AnonymizedAt),
	}, nil
}

func (s *Repository) Identity(ctx context.Context, tenantID, id string) (*identitydomain.Identity, error) {
	row, ok, err := identityrquery.GetIdentity.One(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	return &identitydomain.Identity{
		ID:             row.ID,
		TenantID:       row.TenantID,
		RealmID:        nstr(row.RealmID),
		RealmCode:      nstr(row.RealmCode),
		RealmKind:      nstr(row.RealmKind),
		Username:       row.Username,
		Email:          nstr(row.Email),
		Status:         row.Status,
		AssuranceLevel: int(row.AssuranceLevel),
		TokenEpoch:     int(row.TokenEpoch),
		Disabled:       present(row.DisabledAt),
		Anonymized:     present(row.AnonymizedAt),
	}, nil
}

func (s *Repository) CreateIdentity(ctx context.Context, in identitydomain.IdentityCreate) (string, error) {
	row, _, err := identityrquery.CreateIdentity.One(ctx, s.ex(ctx),
		in.TenantID, optArg(in.RealmID), in.Username, in.Email, in.ExternalRef,
		int16(in.AssuranceLevel), optArg(in.CategoryID), in.Status)
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.ID, nil
}

func (s *Repository) DisableIdentity(ctx context.Context, tenantID, id string) error {
	n, err := identityrquery.DisableIdentity.Exec(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound
	}
	return nil
}

func (s *Repository) EnableIdentity(ctx context.Context, tenantID, id string) error {
	n, err := identityrquery.EnableIdentity.Exec(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound
	}
	return nil
}

// BumpTokenEpoch invalidates every outstanding token for one identity. The
// increment is computed by the database: in Go it is a read-modify-write, and
// two concurrent revocations would leave one set of tokens live.
func (s *Repository) BumpTokenEpoch(ctx context.Context, tenantID, id string) (int, error) {
	row, ok, err := identityrquery.BumpTokenEpoch.One(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	if !ok {
		return 0, apperr.ErrNotFound
	}
	return int(row.TokenEpoch), nil
}

// TouchLastLogin is best effort: a login that worked must not fail on it.
func (s *Repository) TouchLastLogin(ctx context.Context, id string) {
	_, _ = identityrquery.TouchLastLogin.Exec(ctx, s.ex(ctx), id)
}

func (s *Repository) RequestErasure(ctx context.Context, tenantID, id string) error {
	n, err := identityrquery.RequestErasure.Exec(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound
	}
	return nil
}

func (s *Repository) LinkIdentities(ctx context.Context, tenantID, primaryID, secondaryID, linkedBy, method string, evidence []byte) error {
	_, err := identityrquery.LinkIdentities.Exec(ctx, s.ex(ctx),
		tenantID, primaryID, secondaryID, linkedBy, method,
		database.OrEmptyJSON(evidence))
	return database.MapErr(err)
}

// CountIdentitiesByRealm backs the console's overview — one row per
// population, zero rows counted honestly.
func (s *Repository) CountIdentitiesByRealm(ctx context.Context, tenantID string) ([]identitydomain.RealmCount, error) {
	rows, err := identityrquery.CountIdentitiesByRealm.Query(ctx, s.ex(ctx), tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]identitydomain.RealmCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, identitydomain.RealmCount{Realm: r.Realm, Kind: r.Kind, Count: r.N})
	}
	return out, nil
}

// CountRetentionBacklog is the compliance clock: rows past retention_until
// and not yet anonymised.
func (s *Repository) CountRetentionBacklog(ctx context.Context, tenantID string) (int64, error) {
	row, _, err := identityrquery.CountRetentionBacklog.One(ctx, s.ex(ctx), tenantID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	return row.N, nil
}
