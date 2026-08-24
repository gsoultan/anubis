package authpg

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gsoultan/anubis/internal/auth/adapter/postgres/gen"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) CreateAPIKey(ctx context.Context, tenantID, label, lookup, secretHash, createdBy string, expiresAt *int64) (string, error) {
	arg := gen.CreateAPIKeyParams{
		TenantID: tenantID, Label: label, Lookup: lookup, SecretHash: secretHash,
	}
	if createdBy != "" {
		arg.CreatedBy = &createdBy
	}
	if expiresAt != nil {
		t := time.Unix(*expiresAt, 0).UTC()
		arg.ExpiresAt = &t
	}
	id, err := s.q(ctx).CreateAPIKey(ctx, arg)
	if err != nil {
		return "", database.MapErr(err)
	}
	return id, nil
}

func (s *Repository) APIKeyByLookup(ctx context.Context, lookup string) (*authdomain.APIKeyAuth, error) {
	row, err := s.q(ctx).GetAPIKeyByLookup(ctx, lookup)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, database.MapErr(err)
	}
	return &authdomain.APIKeyAuth{
		ID: row.ID, TenantID: row.TenantID, TenantSlug: row.TenantSlug,
		TenantStatus: row.TenantStatus, SecretHash: row.SecretHash,
		ExpiresAt: row.ExpiresAt,
	}, nil
}

func (s *Repository) ListAPIKeys(ctx context.Context, tenantID string) ([]authdomain.APIKeyRecord, error) {
	rows, err := s.q(ctx).ListAPIKeys(ctx, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authdomain.APIKeyRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, authdomain.APIKeyRecord{
			ID: r.ID, TenantID: tenantID, Label: r.Label, Lookup: r.Lookup,
			CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
			LastUsedAt: r.LastUsedAt, ExpiresAt: r.ExpiresAt, RevokedAt: r.RevokedAt,
		})
	}
	return out, nil
}

func (s *Repository) RevokeAPIKey(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).RevokeAPIKey(ctx, gen.RevokeAPIKeyParams{ID: id, TenantID: tenantID})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound.With("api_key", id)
	}
	return nil
}

func (s *Repository) TouchAPIKeyUsed(ctx context.Context, id string) {
	_ = s.q(ctx).TouchAPIKeyUsed(ctx, id)
}
