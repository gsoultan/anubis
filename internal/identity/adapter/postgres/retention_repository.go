package identitypg

import (
	"context"

	identityrquery "github.com/gsoultan/anubis/internal/identity/adapter/postgres/rquery"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// ApplyRealmRetention stamps a retention deadline on identities that have
// none, from their realm's default. Identities that already have one are left
// alone, so an operator's explicit date survives a policy change.
func (s *Repository) ApplyRealmRetention(ctx context.Context) (int64, error) {
	n, err := identityrquery.SetRetentionFromRealm.Exec(ctx, s.ex(ctx))
	return n, database.MapErr(err)
}

// ExpireRetained is the retention sweep: identities past their statutory limit
// are ANONYMISED, not deleted — rows and referential integrity survive for
// audit while authorize() denies from that moment (migrations/0009 gate 1).
//
// The PII keys come back so the caller can shred them; an identity with no key
// contributes none, which is why the third slice can be shorter than the other
// two.
func (s *Repository) ExpireRetained(ctx context.Context) ([]string, []string, []string, error) {
	rows, err := identityrquery.ExpireRetainedIdentities.Query(ctx, s.ex(ctx))
	if err != nil {
		return nil, nil, nil, database.MapErr(err)
	}
	ids := make([]string, 0, len(rows))
	tenants := make([]string, 0, len(rows))
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
		tenants = append(tenants, r.TenantID)
		if k, ok := r.PiiKeyID.Get(); ok {
			keys = append(keys, k)
		}
	}
	return ids, tenants, keys, nil
}

// Anonymize is right-to-erasure execution: blank the direct identifiers and
// bump the epoch so every outstanding token dies, in one statement. It returns
// the PII key id, which the caller crypto-shreds separately.
func (s *Repository) Anonymize(ctx context.Context, tenantID, identityID string) (string, error) {
	row, ok, err := identityrquery.AnonymizeIdentity.One(ctx, s.ex(ctx), identityID, tenantID)
	if err != nil {
		return "", database.MapErr(err)
	}
	if !ok {
		// Already anonymised: the requested outcome is the current state.
		return "", nil
	}
	return nstr(row.PiiKeyID), nil
}
