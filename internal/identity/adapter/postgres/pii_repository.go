package identitypg

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/identity/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) CreatePIIKey(ctx context.Context, tenantID string, sealed []byte, kmsRef string) (string, error) {
	id, err := s.q(ctx).CreatePIIKey(ctx, gen.CreatePIIKeyParams{
		TenantID: tenantID, KeyEnc: sealed, KmsKeyRef: kmsRef,
	})
	return id, database.MapErr(err)
}

func (s *Repository) PIIKey(ctx context.Context, tenantID, id string) ([]byte, error) {
	row, err := s.q(ctx).GetPIIKey(ctx, gen.GetPIIKeyParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return row.KeyEnc, nil
}

func (s *Repository) SetIdentityPIIKey(ctx context.Context, tenantID, identityID, keyID string) error {
	return database.MapErr(s.q(ctx).SetIdentityPIIKey(ctx, gen.SetIdentityPIIKeyParams{
		ID: identityID, TenantID: tenantID, PiiKeyID: database.OptStr(keyID),
	}))
}

func (s *Repository) ShredPIIKey(ctx context.Context, keyID, reason string) (bool, error) {
	ok, err := s.q(ctx).ShredPIIKey(ctx, gen.ShredPIIKeyParams{KeyID: keyID, Reason: reason})
	return ok, database.MapErr(err)
}

func (s *Repository) IdentityAttributes(ctx context.Context, tenantID, identityID string) ([]byte, []byte, string, error) {
	row, err := s.q(ctx).GetIdentityAttributes(ctx, gen.GetIdentityAttributesParams{
		ID: identityID, TenantID: tenantID,
	})
	if err != nil {
		return nil, nil, "", database.MapErr(err)
	}
	return row.Attributes, row.KeyEnc, database.Deref(row.PiiKeyID), nil
}

func (s *Repository) SetIdentityAttributes(ctx context.Context, tenantID, identityID string, envelope []byte) error {
	n, err := s.q(ctx).SetIdentityAttributes(ctx, gen.SetIdentityAttributesParams{
		ID: identityID, TenantID: tenantID, Attributes: envelope,
	})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return database.NotFound()
	}
	return nil
}
