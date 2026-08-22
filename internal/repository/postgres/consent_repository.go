package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) ListConsents(ctx context.Context, tenantID, identityID string) ([]repository.ConsentRecord, error) {
	rows, err := s.q(ctx).ListConsents(ctx, gen.ListConsentsParams{
		IdentityID: identityID, TenantID: tenantID,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.ConsentRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.ConsentRecord{
			ID: r.ID, IdentityID: r.IdentityID, Purpose: r.Purpose,
			PolicyVersion: r.PolicyVersion, GrantedAt: r.GrantedAt,
			WithdrawnAt: r.WithdrawnAt, ExpiresAt: r.ExpiresAt,
		})
	}
	return out, nil
}

func (s *Store) InsertConsent(ctx context.Context, tenantID, identityID, purpose, policyVersion string, evidence []byte) (*repository.ConsentRecord, error) {
	row, err := s.q(ctx).InsertConsent(ctx, gen.InsertConsentParams{
		TenantID: tenantID, IdentityID: identityID, Purpose: purpose,
		PolicyVersion: policyVersion, Evidence: orEmptyJSON(evidence),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.ConsentRecord{
		ID: row.ID, IdentityID: identityID, Purpose: purpose,
		PolicyVersion: policyVersion, GrantedAt: row.GrantedAt,
	}, nil
}

func (s *Store) WithdrawConsent(ctx context.Context, tenantID, id string) error {
	n, err := s.q(ctx).WithdrawConsent(ctx, gen.WithdrawConsentParams{ID: id, TenantID: tenantID})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return notFoundErr()
	}
	return nil
}
