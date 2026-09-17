package identitypg

import (
	"context"

	"github.com/gsoultan/anubis/internal/identity/adapter/postgres/rgen/consent"
	identityrquery "github.com/gsoultan/anubis/internal/identity/adapter/postgres/rquery"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/storm/runtime"
)

// ListConsents is the lawful basis for processing one person's data. Scoped by
// tenant as well as identity — the tenant is what makes the identity id
// somebody's to ask about.
func (s *Repository) ListConsents(ctx context.Context, tenantID, identityID string) ([]identitydomain.ConsentRecord, error) {
	tid, ident, err := twoUUIDs(tenantID, identityID)
	if err != nil {
		return nil, err
	}
	rows, err := consent.New().
		Where(consent.IdentityID.Eq(ident), consent.TenantID.Eq(tid)).
		Order(consent.GrantedAt.Desc()).
		All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]identitydomain.ConsentRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, identitydomain.ConsentRecord{
			ID: database.UUIDStr(r.ID), IdentityID: database.UUIDStr(r.IdentityID),
			Purpose: r.Purpose, PolicyVersion: r.PolicyVersion,
			GrantedAt: r.GrantedAt, WithdrawnAt: tptr(r.WithdrawnAt),
			ExpiresAt: tptr(r.ExpiresAt),
		})
	}
	return out, nil
}

func (s *Repository) InsertConsent(ctx context.Context, tenantID, identityID, purpose, policyVersion string, evidence []byte) (*identitydomain.ConsentRecord, error) {
	tid, ident, err := twoUUIDs(tenantID, identityID)
	if err != nil {
		return nil, err
	}
	n := consent.Create()
	n.SetTenantID(tid)
	n.SetIdentityID(ident)
	n.SetPurpose(purpose)
	n.SetPolicyVersion(policyVersion)
	n.SetEvidence(runtime.JSON(database.OrEmptyJSON(evidence)))
	row, err := n.Insert(ctx, s.ex(ctx))
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &identitydomain.ConsentRecord{
		ID: database.UUIDStr(row.ID), IdentityID: identityID, Purpose: purpose,
		PolicyVersion: policyVersion, GrantedAt: row.GrantedAt,
	}, nil
}

// WithdrawConsent stamps the row it withdraws; the guard makes the row count
// distinguish "I withdrew it" from "it already was".
func (s *Repository) WithdrawConsent(ctx context.Context, tenantID, id string) error {
	n, err := identityrquery.WithdrawConsent.Exec(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return database.NotFound()
	}
	return nil
}
