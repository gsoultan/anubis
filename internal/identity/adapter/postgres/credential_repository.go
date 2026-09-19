package identitypg

import (
	"context"

	"github.com/gsoultan/anubis/internal/identity/adapter/postgres/rgen/credential"
	identityrquery "github.com/gsoultan/anubis/internal/identity/adapter/postgres/rquery"
	credentialdomain "github.com/gsoultan/anubis/internal/identity/domain/credential"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) PasswordCredential(ctx context.Context, identityID string) (*credentialdomain.Credential, error) {
	row, ok, err := identityrquery.GetPasswordCredential.One(ctx, s.ex(ctx), identityID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	return &credentialdomain.Credential{
		ID: row.ID, IdentityID: row.IdentityID, TenantID: row.TenantID,
		Kind: "password", Secret: nstr(row.Secret), Params: []byte(row.Params),
	}, nil
}

func (s *Repository) CreateCredential(ctx context.Context, in credentialdomain.CredentialInput) (string, error) {
	row, _, err := identityrquery.CreateCredential.One(ctx, s.ex(ctx),
		in.IdentityID, in.TenantID, in.Kind, in.Secret, in.LookupKey, in.Label,
		database.OrEmptyJSON(in.Params), database.OptTime(in.ExpiresAt))
	if err != nil {
		return "", database.MapErr(err)
	}
	return row.ID, nil
}

func (s *Repository) RevokeCredential(ctx context.Context, tenantID, id string) error {
	n, err := identityrquery.RevokeCredential.Exec(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound
	}
	return nil
}

func (s *Repository) RevokeCredentialsOfKind(ctx context.Context, identityID, kind string) (int64, error) {
	n, err := identityrquery.RevokeCredentialsOfKind.Exec(ctx, s.ex(ctx), identityID, kind)
	return n, database.MapErr(err)
}

// UpdateCredentialSecret writes the secret and the key it was sealed under.
// An empty kid stores NULL, which is what a password wants: it is hashed, not
// sealed, and has no key to name.
func (s *Repository) UpdateCredentialSecret(ctx context.Context, id, secret, kid string) error {
	_, err := identityrquery.UpdateCredentialSecret.Exec(ctx, s.ex(ctx),
		id, database.OptStr(secret), database.OptStr(kid))
	return database.MapErr(err)
}

// UpdateCredentialParams carries the TOTP replay guard: params holds the last
// accepted time step.
func (s *Repository) UpdateCredentialParams(ctx context.Context, id string, params []byte) error {
	_, err := identityrquery.UpdateCredentialParams.Exec(ctx, s.ex(ctx), id, database.OrEmptyJSON(params))
	return database.MapErr(err)
}

func (s *Repository) ActiveCredentialOfKind(ctx context.Context, identityID, kind string) (*credentialdomain.Credential, error) {
	row, ok, err := identityrquery.GetActiveCredentialOfKind.One(ctx, s.ex(ctx), identityID, kind)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	return &credentialdomain.Credential{
		ID: row.ID, IdentityID: row.IdentityID, TenantID: row.TenantID,
		Kind: row.Kind, Secret: nstr(row.Secret), SecretKid: nstr(row.SecretKid),
		Params: []byte(row.Params), SignCounter: row.SignCounter,
	}, nil
}

func (s *Repository) ActiveCredentialKinds(ctx context.Context, identityID string) ([]string, error) {
	rows, err := identityrquery.ListActiveCredentialKinds.Query(ctx, s.ex(ctx), identityID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Kind)
	}
	return out, nil
}

// TouchCredentialUsed advances the WebAuthn replay guard with GREATEST: an
// authenticator reporting a LOWER counter than we have seen is the signal that
// its credential has been cloned, and writing the reported value would erase
// that evidence. Best effort, like every other usage stamp.
func (s *Repository) TouchCredentialUsed(ctx context.Context, id string, signCounter int64) {
	_, _ = identityrquery.TouchCredentialUsed.Exec(ctx, s.ex(ctx), id, signCounter)
}

// ListCredentials is a tenant-scoped inventory of how one person signs in.
//
// tenant_id is a FILTER, not just a column: it was the second predicate and
// never the first, so an admin RPC carrying somebody else's identity id
// answered with their inventory.
func (s *Repository) ListCredentials(ctx context.Context, tenantID, identityID, kind string) ([]credentialdomain.CredentialInfo, error) {
	ident, err := database.ParseUUID(identityID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	rows, err := credential.New().
		Where(credential.IdentityID.Eq(ident), credential.TenantID.Eq(tid)).
		WhereIf(kind != "", credential.Kind.Eq(kind)).
		Order(credential.CreatedAt.Desc()).
		All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]credentialdomain.CredentialInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, credentialdomain.CredentialInfo{
			ID: database.UUIDStr(r.ID), Kind: r.Kind, Label: nstr(r.Label),
			LookupKey: nstr(r.LookupKey), CreatedAt: r.CreatedAt,
			LastUsedAt: tptr(r.LastUsedAt), ExpiresAt: tptr(r.ExpiresAt),
			RevokedAt: tptr(r.RevokedAt),
		})
	}
	return out, nil
}

// CredentialOwner resolves a credential to its identity, refusing a revoked
// one: a caller acting on it would otherwise be acting on a credential that
// no longer proves anything.
func (s *Repository) CredentialOwner(ctx context.Context, id string) (string, string, string, error) {
	cid, err := database.ParseUUID(id)
	if err != nil {
		return "", "", "", database.MapErr(err)
	}
	row, ok, err := credential.New().Where(credential.ID.Eq(cid)).One(ctx, s.ex(ctx))
	if err != nil {
		return "", "", "", database.MapErr(err)
	}
	if !ok || present(row.RevokedAt) {
		return "", "", "", apperr.ErrNotFound
	}
	return database.UUIDStr(row.IdentityID), database.UUIDStr(row.TenantID), row.Kind, nil
}

// CountActiveCredentialsOfKind backs "you cannot remove your last factor".
func (s *Repository) CountActiveCredentialsOfKind(ctx context.Context, identityID, kind string) (int, error) {
	ident, err := database.ParseUUID(identityID)
	if err != nil {
		return 0, database.MapErr(err)
	}
	n, err := credential.New().
		Where(credential.IdentityID.Eq(ident), credential.Kind.Eq(kind),
			credential.RevokedAt.IsNull()).
		Count(ctx, s.ex(ctx))
	if err != nil {
		return 0, database.MapErr(err)
	}
	return int(n), nil
}

// ConsumeRecoveryCode is single use: the row is revoked AS it is accepted, in
// one statement, so two concurrent presentations cannot both win.
func (s *Repository) ConsumeRecoveryCode(ctx context.Context, identityID, codeHash string) (string, error) {
	row, ok, err := identityrquery.ConsumeRecoveryCode.One(ctx, s.ex(ctx), identityID, codeHash)
	if err != nil {
		return "", database.MapErr(err)
	}
	if !ok {
		return "", apperr.ErrNotFound
	}
	return row.ID, nil
}
