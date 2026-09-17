package controlpg

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/control/adapter/postgres/rgen/platformrefreshtoken"
	controlrquery "github.com/gsoultan/anubis/internal/control/adapter/postgres/rquery"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// CreateRefreshFamily begins a sign-in; the row's own id is the family id,
// which is what lets one revocation kill the session however many times it
// rotated. Only the hash arrives here — the secret never touches this layer.
func (s *Repository) CreateRefreshFamily(ctx context.Context, platformUserID string, tokenHash []byte, expiresAt time.Time) (string, error) {
	row, _, err := controlrquery.InsertPlatformRefreshRoot.One(ctx, s.ex(ctx),
		platformUserID, tokenHash, expiresAt)
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.ID, nil
}

// AppendRefresh adds a rotation's successor to an existing family.
func (s *Repository) AppendRefresh(ctx context.Context, platformUserID, familyID string, tokenHash []byte, expiresAt time.Time) error {
	owner, err := database.ParseUUID(platformUserID)
	if err != nil {
		return database.MapErr(err)
	}
	family, err := database.ParseUUID(familyID)
	if err != nil {
		return database.MapErr(err)
	}
	n := platformrefreshtoken.Create()
	n.SetPlatformUserID(owner)
	n.SetFamilyID(family)
	n.SetTokenHash(tokenHash)
	n.SetExpiresAt(expiresAt)
	if _, err := n.Insert(ctx, s.ex(ctx)); err != nil {
		return database.MapErr(err)
	}
	return nil
}

// RefreshByHash resolves a presented token. nil means no such token, which the
// caller treats as a failed rotation rather than an error.
func (s *Repository) RefreshByHash(ctx context.Context, tokenHash []byte) (*controldomain.PlatformRefresh, error) {
	row, ok, err := controlrquery.PlatformRefreshByHash.One(ctx, s.ex(ctx), tokenHash)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, nil
	}
	return &controldomain.PlatformRefresh{
		ID:             row.ID,
		PlatformUserID: row.PlatformUserID,
		FamilyID:       row.FamilyID,
		CreatedAt:      row.CreatedAt,
		ExpiresAt:      row.ExpiresAt,
		UsedAt:         tptr(row.UsedAt),
		RevokedAt:      tptr(row.RevokedAt),
	}, nil
}

// ConsumeRefresh flips used_at exactly once. A second concurrent presenter
// changes no rows, and reports false — which the interactor reads as reuse,
// not as a race to shrug at.
func (s *Repository) ConsumeRefresh(ctx context.Context, id string) (bool, error) {
	n, err := controlrquery.ConsumePlatformRefresh.Exec(ctx, s.ex(ctx), id)
	if err != nil {
		return false, database.MapErr(err)
	}
	return n > 0, nil
}

func (s *Repository) RevokeRefreshFamily(ctx context.Context, familyID string) error {
	_, err := controlrquery.RevokePlatformRefreshFamily.Exec(ctx, s.ex(ctx), familyID)
	return database.MapErr(err)
}

func (s *Repository) SweepExpired(ctx context.Context) (int64, error) {
	n, err := controlrquery.SweepPlatformRefresh.Exec(ctx, s.ex(ctx))
	if err != nil {
		return 0, database.MapErr(err)
	}
	return n, nil
}
