package authzpg

import (
	"context"
	"encoding/json"

	authzrquery "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rquery"
	"github.com/gsoultan/anubis/internal/authz/domain/share"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) CreateShare(ctx context.Context, in share.CreateInput) (*share.Share, error) {
	scopes, err := json.Marshal(in.RecordScopes)
	if err != nil {
		return nil, apperr.ErrInvalidArgument.Wrap(err)
	}
	row, _, err := authzrquery.CreateShare.One(ctx, s.rex(ctx),
		in.TenantID, in.FromIdentityID, in.ToIdentityID, in.Record,
		string(scopes), in.Permissions, in.Note, in.ValidUntil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return shareFromRow(row), nil
}

func (s *Repository) Share(ctx context.Context, tenantID, id string) (*share.Share, error) {
	row, ok, err := authzrquery.GetShare.One(ctx, s.rex(ctx), id, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, apperr.ErrNotFound
	}
	return shareFromRow(row), nil
}

func (s *Repository) SharesOnRecord(ctx context.Context, tenantID, record string) ([]share.Share, error) {
	rows, err := authzrquery.ListSharesOnRecord.Query(ctx, s.rex(ctx), tenantID, record)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return sharesFromRows(rows), nil
}

func (s *Repository) SharesForRecipient(ctx context.Context, tenantID, identityID string) ([]share.Share, error) {
	rows, err := authzrquery.ListSharesForRecipient.Query(ctx, s.rex(ctx), tenantID, identityID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return sharesFromRows(rows), nil
}

func (s *Repository) RevokeShare(ctx context.Context, tenantID, id, reason string) (bool, error) {
	n, err := authzrquery.RevokeShare.Exec(ctx, s.rex(ctx), id, tenantID, reason)
	if err != nil {
		return false, database.MapErr(err)
	}
	return n > 0, nil
}

func sharesFromRows(rows []authzrquery.ShareRow) []share.Share {
	out := make([]share.Share, 0, len(rows))
	for i := range rows {
		out = append(out, *shareFromRow(rows[i]))
	}
	return out
}

func shareFromRow(r authzrquery.ShareRow) *share.Share {
	sh := &share.Share{
		ID: r.ID, TenantID: r.TenantID, FromIdentityID: r.FromIdentityID, ToIdentityID: r.ToIdentityID,
		Record: r.Record, Permissions: r.PermissionKeys, Note: r.Note,
		CreatedAt: r.CreatedAt, ValidUntil: r.ValidUntil,
		RevokedAt: r.RevokedAt.Ptr(), RevokeReason: r.RevokeReason.V,
		RecordScopes: map[string]string{},
	}
	// A stored scope map that does not parse is a wiring bug, not a reason
	// to drop the share: the map is advisory for later authorisation, and an
	// empty map fails that authorisation closed.
	_ = json.Unmarshal([]byte(r.RecordScopes), &sh.RecordScopes)
	return sh
}
