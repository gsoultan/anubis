package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) PasswordCredential(ctx context.Context, identityID string) (*repository.Credential, error) {
	row, err := s.q(ctx).GetPasswordCredential(ctx, identityID)
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.Credential{
		ID: row.ID, IdentityID: row.IdentityID, TenantID: row.TenantID,
		Kind: "password", Secret: deref(row.Secret), Params: row.Params,
	}, nil
}

func (s *Store) CreateCredential(ctx context.Context, in repository.CredentialInput) (string, error) {
	row, err := s.q(ctx).CreateCredential(ctx, gen.CreateCredentialParams{
		IdentityID: in.IdentityID, TenantID: in.TenantID, Kind: in.Kind,
		Secret: in.Secret, LookupKey: in.LookupKey, Label: in.Label,
		Params: orEmptyJSON(in.Params), ExpiresAt: optTime(in.ExpiresAt),
	})
	if err != nil {
		return "", mapErr(err)
	}
	return row.ID, nil
}

func (s *Store) RevokeCredential(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).RevokeCredential(ctx, gen.RevokeCredentialParams{ID: id, TenantID: tenantID})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) RevokeCredentialsOfKind(ctx context.Context, identityID, kind string) (int64, error) {
	n, err := s.q(ctx).RevokeCredentialsOfKind(ctx, gen.RevokeCredentialsOfKindParams{
		IdentityID: identityID, Kind: kind,
	})
	return n, mapErr(err)
}

func (s *Store) UpdateCredentialSecret(ctx context.Context, id, secret string) error {
	return mapErr(s.q(ctx).UpdateCredentialSecret(ctx, gen.UpdateCredentialSecretParams{
		ID: id, Secret: optStr(secret),
	}))
}

func (s *Store) UpdateCredentialParams(ctx context.Context, id string, params []byte) error {
	return mapErr(s.q(ctx).UpdateCredentialParams(ctx, gen.UpdateCredentialParamsParams{
		ID: id, Params: params,
	}))
}

func (s *Store) ActiveCredentialOfKind(ctx context.Context, identityID, kind string) (*repository.Credential, error) {
	row, err := s.q(ctx).GetActiveCredentialOfKind(ctx, gen.GetActiveCredentialOfKindParams{
		IdentityID: identityID, Kind: kind,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.Credential{
		ID: row.ID, IdentityID: row.IdentityID, TenantID: row.TenantID,
		Kind: row.Kind, Secret: deref(row.Secret), Params: row.Params,
		SignCounter: row.SignCounter,
	}, nil
}

func (s *Store) ActiveCredentialKinds(ctx context.Context, identityID string) ([]string, error) {
	kinds, err := s.q(ctx).ListActiveCredentialKinds(ctx, identityID)
	return kinds, mapErr(err)
}

func (s *Store) CredentialByLookup(ctx context.Context, lookupKey string) (*repository.APIKeyCredential, error) {
	row, err := s.q(ctx).GetCredentialByLookup(ctx, optStr(lookupKey))
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.APIKeyCredential{
		ID: row.ID, IdentityID: row.IdentityID, TenantID: row.TenantID,
		SecretHash: deref(row.Secret), ExpiresAt: row.ExpiresAt,
		IdentityStatus: row.IdentityStatus, TokenEpoch: int(row.TokenEpoch),
		Blocked: row.DisabledAt != nil || row.AnonymizedAt != nil,
	}, nil
}

func (s *Store) TouchCredentialUsed(ctx context.Context, id string, signCounter int64) {
	_ = s.q(ctx).TouchCredentialUsed(ctx, gen.TouchCredentialUsedParams{
		ID: id, SignCounter: signCounter,
	})
}

func (s *Store) ListCredentials(ctx context.Context, identityID, kind string) ([]repository.CredentialInfo, error) {
	rows, err := s.q(ctx).ListCredentials(ctx, gen.ListCredentialsParams{
		IdentityID: identityID, Kind: optStr(kind),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.CredentialInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.CredentialInfo{
			ID: r.ID, Kind: r.Kind, Label: deref(r.Label),
			LookupKey: deref(r.LookupKey), CreatedAt: r.CreatedAt,
			LastUsedAt: r.LastUsedAt, ExpiresAt: r.ExpiresAt, RevokedAt: r.RevokedAt,
		})
	}
	return out, nil
}

func (s *Store) CredentialOwner(ctx context.Context, id string) (string, string, string, error) {
	row, err := s.q(ctx).GetCredential(ctx, id)
	if err != nil {
		return "", "", "", mapErr(err)
	}
	if row.RevokedAt != nil {
		return "", "", "", domain.ErrNotFound
	}
	return row.IdentityID, row.TenantID, row.Kind, nil
}

func orEmptyJSON(b []byte) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}
