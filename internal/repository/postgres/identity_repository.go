package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) IdentityForLogin(ctx context.Context, tenantID, realmID, username string) (*domain.Identity, error) {
	row, err := s.q(ctx).GetIdentityForLogin(ctx, gen.GetIdentityForLoginParams{
		TenantID: tenantID, RealmID: optStr(realmID), Username: username,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return &domain.Identity{
		ID:             row.ID,
		TenantID:       row.TenantID,
		RealmID:        deref(row.RealmID),
		RealmCode:      deref(row.RealmCode),
		RealmKind:      deref(row.RealmKind),
		Username:       row.Username,
		Email:          deref(row.Email),
		Status:         row.Status,
		AssuranceLevel: int(row.AssuranceLevel),
		TokenEpoch:     int(row.TokenEpoch),
		Disabled:       row.DisabledAt != nil,
		Anonymized:     row.AnonymizedAt != nil,
	}, nil
}

func (s *Store) Identity(ctx context.Context, tenantID, id string) (*domain.Identity, error) {
	row, err := s.q(ctx).GetIdentity(ctx, gen.GetIdentityParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, mapErr(err)
	}
	return &domain.Identity{
		ID:             row.ID,
		TenantID:       row.TenantID,
		RealmID:        deref(row.RealmID),
		RealmCode:      deref(row.RealmCode),
		RealmKind:      deref(row.RealmKind),
		Username:       row.Username,
		Email:          deref(row.Email),
		Status:         row.Status,
		AssuranceLevel: int(row.AssuranceLevel),
		TokenEpoch:     int(row.TokenEpoch),
		Disabled:       row.DisabledAt != nil,
		Anonymized:     row.AnonymizedAt != nil,
	}, nil
}

func (s *Store) CreateIdentity(ctx context.Context, in repository.IdentityCreate) (string, error) {
	row, err := s.q(ctx).CreateIdentity(ctx, gen.CreateIdentityParams{
		TenantID:       in.TenantID,
		RealmID:        optStr(in.RealmID),
		Username:       in.Username,
		Email:          in.Email,
		ExternalRef:    in.ExternalRef,
		AssuranceLevel: int16(in.AssuranceLevel),
		CategoryID:     optStr(in.CategoryID),
		Status:         in.Status,
	})
	if err != nil {
		return "", mapErr(err)
	}
	return row.ID, nil
}

func (s *Store) DisableIdentity(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).DisableIdentity(ctx, gen.DisableIdentityParams{ID: id, TenantID: tenantID})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) EnableIdentity(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).EnableIdentity(ctx, gen.EnableIdentityParams{ID: id, TenantID: tenantID})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) BumpTokenEpoch(ctx context.Context, tenantID, id string) (int, error) {
	epoch, err := s.q(ctx).BumpTokenEpoch(ctx, gen.BumpTokenEpochParams{ID: id, TenantID: tenantID})
	if err != nil {
		return 0, mapErr(err)
	}
	return int(epoch), nil
}

func (s *Store) TouchLastLogin(ctx context.Context, id string) {
	_ = s.q(ctx).TouchLastLogin(ctx, id) // best-effort; login must not fail on it
}

func (s *Store) RequestErasure(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).RequestErasure(ctx, gen.RequestErasureParams{ID: id, TenantID: tenantID})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) LinkIdentities(ctx context.Context, tenantID, primaryID, secondaryID, linkedBy, method string, evidence []byte) error {
	return mapErr(s.q(ctx).LinkIdentities(ctx, gen.LinkIdentitiesParams{
		TenantID: tenantID, PrimaryID: primaryID, SecondaryID: secondaryID,
		LinkedBy: linkedBy, Method: method, Evidence: evidence,
	}))
}
