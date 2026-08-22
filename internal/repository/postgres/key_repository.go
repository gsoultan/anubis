package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) VerificationKeys(ctx context.Context) ([]repository.KeyRecord, error) {
	rows, err := s.q(ctx).ListVerificationKeys(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.KeyRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.KeyRecord{
			Kid: r.Kid, Alg: r.Alg, Status: r.Status, Purpose: r.Purpose,
			PublicKey: r.PublicKey, PrivateKeyEnc: r.PrivateKeyEnc,
			NotBefore: r.NotBefore, NotAfter: r.NotAfter, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

func (s *Store) CreateKey(ctx context.Context, k repository.KeyRecord) error {
	_, err := s.q(ctx).CreateSigningKey(ctx, gen.CreateSigningKeyParams{
		Kid: k.Kid, Alg: k.Alg, Status: k.Status, Purpose: k.Purpose,
		PublicKey: k.PublicKey, PrivateKeyEnc: k.PrivateKeyEnc,
		NotBefore: k.NotBefore, NotAfter: k.NotAfter,
	})
	return mapErr(err)
}

func (s *Store) PromotePending(ctx context.Context, purpose string) (int64, error) {
	n, err := s.q(ctx).PromotePendingKey(ctx, purpose)
	return n, mapErr(err)
}

func (s *Store) DemoteActive(ctx context.Context, purpose string) (int64, error) {
	n, err := s.q(ctx).DemoteActiveKey(ctx, purpose)
	return n, mapErr(err)
}

func (s *Store) SetKeyStatus(ctx context.Context, kid, status string) error {
	_, err := s.q(ctx).SetSigningKeyStatus(ctx, gen.SetSigningKeyStatusParams{
		Kid: kid, Status: status,
	})
	return mapErr(err)
}

func (s *Store) SigningKeys(ctx context.Context) ([]repository.KeyRecord, error) {
	rows, err := s.q(ctx).ListSigningKeys(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.KeyRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.KeyRecord{
			Kid: r.Kid, Alg: r.Alg, Status: r.Status, Purpose: r.Purpose,
			PublicKey: r.PublicKey, NotBefore: r.NotBefore, NotAfter: r.NotAfter,
			CreatedAt: r.CreatedAt, RetiredAt: r.RetiredAt,
		})
	}
	return out, nil
}
