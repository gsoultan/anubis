package identitypg

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/identity/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/identity/domain/credential"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) PasswordCredential(ctx context.Context, identityID string) (*credential.Credential, error) {
	row, err := s.q(ctx).GetPasswordCredential(ctx, identityID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &credential.Credential{
		ID: row.ID, IdentityID: row.IdentityID, TenantID: row.TenantID,
		Kind: "password", Secret: database.Deref(row.Secret), Params: row.Params,
	}, nil
}

func (s *Repository) CreateCredential(ctx context.Context, in credential.CredentialInput) (string, error) {
	row, err := s.q(ctx).CreateCredential(ctx, gen.CreateCredentialParams{
		IdentityID: in.IdentityID, TenantID: in.TenantID, Kind: in.Kind,
		Secret: in.Secret, LookupKey: in.LookupKey, Label: in.Label,
		Params: database.OrEmptyJSON(in.Params), ExpiresAt: database.OptTime(in.ExpiresAt),
	})
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.ID, nil
}

func (s *Repository) RevokeCredential(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).RevokeCredential(ctx, gen.RevokeCredentialParams{ID: id, TenantID: tenantID})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound
	}
	return nil
}

func (s *Repository) RevokeCredentialsOfKind(ctx context.Context, identityID, kind string) (int64, error) {
	n, err := s.q(ctx).RevokeCredentialsOfKind(ctx, gen.RevokeCredentialsOfKindParams{
		IdentityID: identityID, Kind: kind,
	})
	return n, database.MapErr(err)
}

func (s *Repository) UpdateCredentialSecret(ctx context.Context, id, secret string) error {
	return database.MapErr(s.q(ctx).UpdateCredentialSecret(ctx, gen.UpdateCredentialSecretParams{
		ID: id, Secret: database.OptStr(secret),
	}))
}

func (s *Repository) UpdateCredentialParams(ctx context.Context, id string, params []byte) error {
	return database.MapErr(s.q(ctx).UpdateCredentialParams(ctx, gen.UpdateCredentialParamsParams{
		ID: id, Params: params,
	}))
}

func (s *Repository) ActiveCredentialOfKind(ctx context.Context, identityID, kind string) (*credential.Credential, error) {
	row, err := s.q(ctx).GetActiveCredentialOfKind(ctx, gen.GetActiveCredentialOfKindParams{
		IdentityID: identityID, Kind: kind,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &credential.Credential{
		ID: row.ID, IdentityID: row.IdentityID, TenantID: row.TenantID,
		Kind: row.Kind, Secret: database.Deref(row.Secret), Params: row.Params,
		SignCounter: row.SignCounter,
	}, nil
}

func (s *Repository) ActiveCredentialKinds(ctx context.Context, identityID string) ([]string, error) {
	kinds, err := s.q(ctx).ListActiveCredentialKinds(ctx, identityID)
	return kinds, database.MapErr(err)
}

func (s *Repository) TouchCredentialUsed(ctx context.Context, id string, signCounter int64) {
	_ = s.q(ctx).TouchCredentialUsed(ctx, gen.TouchCredentialUsedParams{
		ID: id, SignCounter: signCounter,
	})
}

func (s *Repository) ListCredentials(ctx context.Context, identityID, kind string) ([]credential.CredentialInfo, error) {
	rows, err := s.q(ctx).ListCredentials(ctx, gen.ListCredentialsParams{
		IdentityID: identityID, Kind: database.OptStr(kind),
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]credential.CredentialInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, credential.CredentialInfo{
			ID: r.ID, Kind: r.Kind, Label: database.Deref(r.Label),
			LookupKey: database.Deref(r.LookupKey), CreatedAt: r.CreatedAt,
			LastUsedAt: r.LastUsedAt, ExpiresAt: r.ExpiresAt, RevokedAt: r.RevokedAt,
		})
	}
	return out, nil
}

func (s *Repository) CredentialOwner(ctx context.Context, id string) (string, string, string, error) {
	row, err := s.q(ctx).GetCredential(ctx, id)
	if err != nil {
		return "", "", "", database.MapErr(err)
	}
	if row.RevokedAt != nil {
		return "", "", "", apperr.ErrNotFound
	}
	return row.IdentityID, row.TenantID, row.Kind, nil
}
