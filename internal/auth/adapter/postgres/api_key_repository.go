package authpg

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/auth/adapter/postgres/rgen/apikey"
	authrquery "github.com/gsoultan/anubis/internal/auth/adapter/postgres/rquery"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) CreateAPIKey(ctx context.Context, tenantID, label, lookup, secretHash, createdBy string, expiresAt *int64) (string, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return "", database.MapErr(err)
	}
	n := apikey.Create()
	n.SetTenantID(tid)
	n.SetLabel(label)
	n.SetLookup(lookup)
	n.SetSecretHash(secretHash)
	if createdBy == "" {
		// Null, not the zero uuid: a key minted by the bootstrap has no
		// creator, which is a different fact from one created by nobody.
		n.SetCreatedByNull()
	} else {
		by, err := database.ParseUUID(createdBy)
		if err != nil {
			return "", database.MapErr(err)
		}
		n.SetCreatedBy(by)
	}
	if expiresAt == nil {
		n.SetExpiresAtNull()
	} else {
		n.SetExpiresAt(time.Unix(*expiresAt, 0).UTC())
	}
	row, err := n.Insert(ctx, s.ex(ctx))
	if err != nil {
		return "", database.MapErr(err)
	}
	return database.UUIDStr(row.ID), nil
}

// APIKeyByLookup resolves a presented key with its tenant's current status.
// nil means no live key, which the caller treats as a failed authentication
// rather than an error.
func (s *Repository) APIKeyByLookup(ctx context.Context, lookup string) (*authdomain.APIKeyAuth, error) {
	row, ok, err := authrquery.GetAPIKeyByLookup.One(ctx, s.ex(ctx), lookup)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, nil
	}
	return &authdomain.APIKeyAuth{
		ID: row.ID, TenantID: row.TenantID, TenantSlug: row.TenantSlug,
		TenantStatus: row.TenantStatus, SecretHash: row.SecretHash,
		ExpiresAt: tptr(row.ExpiresAt),
	}, nil
}

func (s *Repository) ListAPIKeys(ctx context.Context, tenantID string) ([]authdomain.APIKeyRecord, error) {
	rows, err := authrquery.ListAPIKeys.Query(ctx, s.ex(ctx), tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authdomain.APIKeyRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, authdomain.APIKeyRecord{
			ID: r.ID, TenantID: tenantID, Label: r.Label, Lookup: r.Lookup,
			CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
			LastUsedAt: tptr(r.LastUsedAt), ExpiresAt: tptr(r.ExpiresAt),
			RevokedAt: tptr(r.RevokedAt),
		})
	}
	return out, nil
}

func (s *Repository) RevokeAPIKey(ctx context.Context, tenantID, id string) error {
	n, err := authrquery.RevokeAPIKey.Exec(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound.With("api_key", id)
	}
	return nil
}

// TouchAPIKeyUsed is best effort: a request that authenticated must not fail
// because its usage timestamp did not land.
func (s *Repository) TouchAPIKeyUsed(ctx context.Context, id string) {
	kid, err := database.ParseUUID(id)
	if err != nil {
		return // not an id, so there is no row to touch
	}
	m := apikey.MutateKey(kid)
	m.SetLastUsedAtNow()
	_ = m.Update(ctx, s.ex(ctx))
}
