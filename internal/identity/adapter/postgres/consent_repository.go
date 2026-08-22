package identitypg

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/identity/adapter/postgres/gen"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) ListConsents(ctx context.Context, tenantID, identityID string) ([]identitydomain.ConsentRecord, error) {
	rows, err := s.q(ctx).ListConsents(ctx, gen.ListConsentsParams{
		IdentityID: identityID, TenantID: tenantID,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]identitydomain.ConsentRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, identitydomain.ConsentRecord{
			ID: r.ID, IdentityID: r.IdentityID, Purpose: r.Purpose,
			PolicyVersion: r.PolicyVersion, GrantedAt: r.GrantedAt,
			WithdrawnAt: r.WithdrawnAt, ExpiresAt: r.ExpiresAt,
		})
	}
	return out, nil
}

func (s *Repository) InsertConsent(ctx context.Context, tenantID, identityID, purpose, policyVersion string, evidence []byte) (*identitydomain.ConsentRecord, error) {
	row, err := s.q(ctx).InsertConsent(ctx, gen.InsertConsentParams{
		TenantID: tenantID, IdentityID: identityID, Purpose: purpose,
		PolicyVersion: policyVersion, Evidence: database.OrEmptyJSON(evidence),
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &identitydomain.ConsentRecord{
		ID: row.ID, IdentityID: identityID, Purpose: purpose,
		PolicyVersion: policyVersion, GrantedAt: row.GrantedAt,
	}, nil
}

func (s *Repository) WithdrawConsent(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).WithdrawConsent(ctx, gen.WithdrawConsentParams{ID: id, TenantID: tenantID})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return database.NotFound()
	}
	return nil
}
