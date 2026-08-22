package identitypg

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/identity/adapter/postgres/gen"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) IdentityForLogin(ctx context.Context, tenantID, realmID, username string) (*identitydomain.Identity, error) {
	row, err := s.q(ctx).GetIdentityForLogin(ctx, gen.GetIdentityForLoginParams{
		TenantID: tenantID, RealmID: database.OptStr(realmID), Username: username,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &identitydomain.Identity{
		ID:             row.ID,
		TenantID:       row.TenantID,
		RealmID:        database.Deref(row.RealmID),
		RealmCode:      database.Deref(row.RealmCode),
		RealmKind:      database.Deref(row.RealmKind),
		Username:       row.Username,
		Email:          database.Deref(row.Email),
		Status:         row.Status,
		AssuranceLevel: int(row.AssuranceLevel),
		TokenEpoch:     int(row.TokenEpoch),
		Disabled:       row.DisabledAt != nil,
		Anonymized:     row.AnonymizedAt != nil,
	}, nil
}

func (s *Repository) Identity(ctx context.Context, tenantID, id string) (*identitydomain.Identity, error) {
	row, err := s.q(ctx).GetIdentity(ctx, gen.GetIdentityParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &identitydomain.Identity{
		ID:             row.ID,
		TenantID:       row.TenantID,
		RealmID:        database.Deref(row.RealmID),
		RealmCode:      database.Deref(row.RealmCode),
		RealmKind:      database.Deref(row.RealmKind),
		Username:       row.Username,
		Email:          database.Deref(row.Email),
		Status:         row.Status,
		AssuranceLevel: int(row.AssuranceLevel),
		TokenEpoch:     int(row.TokenEpoch),
		Disabled:       row.DisabledAt != nil,
		Anonymized:     row.AnonymizedAt != nil,
	}, nil
}

func (s *Repository) CreateIdentity(ctx context.Context, in identitydomain.IdentityCreate) (string, error) {
	row, err := s.q(ctx).CreateIdentity(ctx, gen.CreateIdentityParams{
		TenantID:       in.TenantID,
		RealmID:        database.OptStr(in.RealmID),
		Username:       in.Username,
		Email:          in.Email,
		ExternalRef:    in.ExternalRef,
		AssuranceLevel: int16(in.AssuranceLevel),
		CategoryID:     database.OptStr(in.CategoryID),
		Status:         in.Status,
	})
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.ID, nil
}

func (s *Repository) DisableIdentity(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).DisableIdentity(ctx, gen.DisableIdentityParams{ID: id, TenantID: tenantID})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound
	}
	return nil
}

func (s *Repository) EnableIdentity(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).EnableIdentity(ctx, gen.EnableIdentityParams{ID: id, TenantID: tenantID})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound
	}
	return nil
}

func (s *Repository) BumpTokenEpoch(ctx context.Context, tenantID, id string) (int, error) {
	epoch, err := s.q(ctx).BumpTokenEpoch(ctx, gen.BumpTokenEpochParams{ID: id, TenantID: tenantID})
	if err != nil {
		return 0, database.MapErr(err)
	}
	return int(epoch), nil
}

func (s *Repository) TouchLastLogin(ctx context.Context, id string) {
	_ = s.q(ctx).TouchLastLogin(ctx, id) // best-effort; login must not fail on it
}

func (s *Repository) RequestErasure(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).RequestErasure(ctx, gen.RequestErasureParams{ID: id, TenantID: tenantID})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound
	}
	return nil
}

func (s *Repository) LinkIdentities(ctx context.Context, tenantID, primaryID, secondaryID, linkedBy, method string, evidence []byte) error {
	return database.MapErr(s.q(ctx).LinkIdentities(ctx, gen.LinkIdentitiesParams{
		TenantID: tenantID, PrimaryID: primaryID, SecondaryID: secondaryID,
		LinkedBy: linkedBy, Method: method, Evidence: evidence,
	}))
}
