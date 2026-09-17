package controlpg

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/control/adapter/postgres/rgen/platformapikey"
	controlrquery "github.com/gsoultan/anubis/internal/control/adapter/postgres/rquery"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// CreatePlatformAPIKey mints a key. Only the hash arrives here; the secret
// itself is never written and never logged.
func (s *Repository) CreatePlatformAPIKey(ctx context.Context, ownerID, label, lookup, secretHash, createdBy string, expiresAt time.Time) (string, error) {
	owner, err := database.ParseUUID(ownerID)
	if err != nil {
		return "", database.MapErr(err)
	}
	n := platformapikey.Create()
	n.SetPlatformUserID(owner)
	n.SetLabel(label)
	n.SetLookup(lookup)
	n.SetSecretHash(secretHash)
	n.SetExpiresAt(expiresAt)
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
	row, err := n.Insert(ctx, s.ex(ctx))
	if err != nil {
		return "", database.MapErr(err)
	}
	return database.UUIDStr(row.ID), nil
}

// PlatformAPIKeyByLookup resolves a presented key, with the owner's current
// status — nil when there is no live key, which the caller treats as a failed
// authentication rather than an error.
func (s *Repository) PlatformAPIKeyByLookup(ctx context.Context, lookup string) (*controldomain.PlatformAPIKeyAuth, error) {
	row, ok, err := controlrquery.PlatformAPIKeyByLookup.One(ctx, s.ex(ctx), lookup)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, nil
	}
	return &controldomain.PlatformAPIKeyAuth{
		ID: row.ID, PlatformUserID: row.PlatformUserID, Username: row.Username,
		SecretHash: row.SecretHash, ExpiresAt: row.ExpiresAt,
		OwnerStatus: row.OwnerStatus, TokenEpoch: int(row.TokenEpoch),
	}, nil
}

func (s *Repository) ListPlatformAPIKeys(ctx context.Context) ([]controldomain.PlatformAPIKey, error) {
	rows, err := controlrquery.ListPlatformAPIKeys.Query(ctx, s.ex(ctx))
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]controldomain.PlatformAPIKey, 0, len(rows))
	for _, r := range rows {
		out = append(out, controldomain.PlatformAPIKey{
			ID: r.ID, PlatformUserID: r.PlatformUserID, Username: r.Username,
			Label: r.Label, Lookup: r.Lookup, CreatedAt: r.CreatedAt,
			LastUsedAt: tptr(r.LastUsedAt), ExpiresAt: r.ExpiresAt,
			RevokedAt: tptr(r.RevokedAt),
		})
	}
	return out, nil
}

// TouchPlatformAPIKeyUsed records a use. Best effort and returns nothing: a
// request that authenticated must not fail because its usage timestamp did not
// land.
func (s *Repository) TouchPlatformAPIKeyUsed(ctx context.Context, id string) {
	_, _ = controlrquery.TouchPlatformAPIKeyUsed.Exec(ctx, s.ex(ctx), id)
}

func (s *Repository) RevokePlatformAPIKey(ctx context.Context, id string) error {
	_, err := controlrquery.RevokePlatformAPIKey.Exec(ctx, s.ex(ctx), id)
	return database.MapErr(err)
}
