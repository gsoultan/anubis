package identitypg

import (
	"context"

	"github.com/gsoultan/anubis/internal/identity/adapter/postgres/rgen/piikey"
	identityrquery "github.com/gsoultan/anubis/internal/identity/adapter/postgres/rquery"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) CreatePIIKey(ctx context.Context, tenantID string, sealed []byte, kmsRef string) (string, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return "", database.MapErr(err)
	}
	n := piikey.Create()
	n.SetTenantID(tid)
	n.SetKeyEnc(sealed)
	if kmsRef == "" {
		// NULL, not '': a key held locally has no KMS reference, which is a
		// different fact from one whose reference is the empty string.
		n.SetKmsKeyRefNull()
	} else {
		n.SetKmsKeyRef(kmsRef)
	}
	row, err := n.Insert(ctx, s.ex(ctx))
	if err != nil {
		return "", database.MapErr(err)
	}
	return database.UUIDStr(row.ID), nil
}

func (s *Repository) PIIKey(ctx context.Context, tenantID, id string) ([]byte, error) {
	tid, kid, err := twoUUIDs(tenantID, id)
	if err != nil {
		return nil, err
	}
	row, ok, err := piikey.New().
		Where(piikey.ID.Eq(kid), piikey.TenantID.Eq(tid)).
		One(ctx, s.ex(ctx))
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	return row.KeyEnc, nil
}

func twoUUIDs(a, b string) ([16]byte, [16]byte, error) {
	x, err := database.ParseUUID(a)
	if err != nil {
		return x, x, database.MapErr(err)
	}
	y, err := database.ParseUUID(b)
	if err != nil {
		return x, y, database.MapErr(err)
	}
	return x, y, nil
}

func (s *Repository) SetIdentityPIIKey(ctx context.Context, tenantID, identityID, keyID string) error {
	_, err := identityrquery.SetIdentityPIIKey.Exec(ctx, s.ex(ctx),
		identityID, tenantID, optArg(keyID))
	return database.MapErr(err)
}

// ShredPIIKey destroys the key an identity's attributes are sealed with.
//
// Idempotent: a second erasure of the same identity returns false rather than
// failing, because "already unrecoverable" is the requested outcome.
func (s *Repository) ShredPIIKey(ctx context.Context, keyID, reason string) (bool, error) {
	row, _, err := identityrquery.ShredPIIKey.One(ctx, s.ex(ctx), keyID, reason)
	if err != nil {
		return false, database.MapErr(err)
	}
	return row.Shredded, nil
}

// IdentityAttributes reads the sealed envelope and the key that opens it
// TOGETHER.
//
// Two statements would leave a window in which retention shreds the key in
// between, and the caller then reports "corrupt" for what is actually a
// completed erasure.
func (s *Repository) IdentityAttributes(ctx context.Context, tenantID, identityID string) ([]byte, []byte, string, error) {
	row, ok, err := identityrquery.GetIdentityAttributes.One(ctx, s.ex(ctx), identityID, tenantID)
	if err != nil {
		return nil, nil, "", database.MapErr(err)
	}
	if !ok {
		return nil, nil, "", database.NotFound()
	}
	return []byte(row.Attributes), row.KeyEnc, nstr(row.PiiKeyID), nil
}

func (s *Repository) SetIdentityAttributes(ctx context.Context, tenantID, identityID string, envelope []byte) error {
	n, err := identityrquery.SetIdentityAttributes.Exec(ctx, s.ex(ctx),
		identityID, tenantID, database.OrEmptyJSON(envelope))
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return database.NotFound()
	}
	return nil
}
