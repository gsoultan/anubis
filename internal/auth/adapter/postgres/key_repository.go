package authpg

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/auth/adapter/postgres/gen"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) VerificationKeys(ctx context.Context) ([]authdomain.KeyRecord, error) {
	rows, err := s.q(ctx).ListVerificationKeys(ctx)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authdomain.KeyRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, authdomain.KeyRecord{
			Kid: r.Kid, Alg: r.Alg, Status: r.Status, Purpose: r.Purpose,
			PublicKey: r.PublicKey, PrivateKeyEnc: r.PrivateKeyEnc,
			NotBefore: r.NotBefore, NotAfter: r.NotAfter, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

func (s *Repository) CreateKey(ctx context.Context, k authdomain.KeyRecord) error {
	_, err := s.q(ctx).CreateSigningKey(ctx, gen.CreateSigningKeyParams{
		Kid: k.Kid, Alg: k.Alg, Status: k.Status, Purpose: k.Purpose,
		PublicKey: k.PublicKey, PrivateKeyEnc: k.PrivateKeyEnc,
		NotBefore: k.NotBefore, NotAfter: k.NotAfter,
	})
	return database.MapErr(err)
}

func (s *Repository) PromotePending(ctx context.Context, purpose string) (int64, error) {
	n, err := s.q(ctx).PromotePendingKey(ctx, purpose)
	return n, database.MapErr(err)
}

func (s *Repository) DemoteActive(ctx context.Context, purpose string) (int64, error) {
	n, err := s.q(ctx).DemoteActiveKey(ctx, purpose)
	return n, database.MapErr(err)
}

func (s *Repository) SetKeyStatus(ctx context.Context, kid, status string) error {
	_, err := s.q(ctx).SetSigningKeyStatus(ctx, gen.SetSigningKeyStatusParams{
		Kid: kid, Status: status,
	})
	return database.MapErr(err)
}

func (s *Repository) SigningKeys(ctx context.Context) ([]authdomain.KeyRecord, error) {
	rows, err := s.q(ctx).ListSigningKeys(ctx)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authdomain.KeyRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, authdomain.KeyRecord{
			Kid: r.Kid, Alg: r.Alg, Status: r.Status, Purpose: r.Purpose,
			PublicKey: r.PublicKey, NotBefore: r.NotBefore, NotAfter: r.NotAfter,
			CreatedAt: r.CreatedAt, RetiredAt: r.RetiredAt,
		})
	}
	return out, nil
}
