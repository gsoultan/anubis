package authpg

import (
	"context"

	"github.com/gsoultan/anubis/internal/auth/adapter/postgres/rgen/signingkey"
	authrquery "github.com/gsoultan/anubis/internal/auth/adapter/postgres/rquery"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// VerificationKeys is everything the keyring preloads: pending (published
// before use), active, and retiring (still verifies until the last token
// signed with it expires).
//
// All three, not just the active one: a token signed a minute before a
// rotation has to keep verifying, and one signed a minute after it has to
// verify on a replica that has not reloaded yet.
func (s *Repository) VerificationKeys(ctx context.Context) ([]authdomain.KeyRecord, error) {
	rows, err := signingkey.New().
		Where(signingkey.Status.In("pending", "active", "retiring")).
		Order(signingkey.CreatedAt.Asc()).
		All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authdomain.KeyRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, keyRecord(r, true))
	}
	return out, nil
}

// SigningKeys is the console's listing. The sealed private half is left out:
// nothing on a screen needs it.
func (s *Repository) SigningKeys(ctx context.Context) ([]authdomain.KeyRecord, error) {
	rows, err := signingkey.New().
		Order(signingkey.CreatedAt.Desc()).
		All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authdomain.KeyRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, keyRecord(r, false))
	}
	return out, nil
}

// keyRecord maps a row; withPrivate says whether the caller is the keyring
// (which needs the sealed half to sign) or a screen (which must not have it).
func keyRecord(r signingkey.Row, withPrivate bool) authdomain.KeyRecord {
	rec := authdomain.KeyRecord{
		Kid: r.Kid, Alg: r.Alg, Status: r.Status, Purpose: r.Purpose,
		PublicKey: r.PublicKey, NotBefore: r.NotBefore, NotAfter: r.NotAfter,
		CreatedAt: r.CreatedAt, RetiredAt: tptr(r.RetiredAt),
	}
	if withPrivate {
		rec.PrivateKeyEnc = r.PrivateKeyEnc
	}
	return rec
}

func (s *Repository) CreateKey(ctx context.Context, k authdomain.KeyRecord) error {
	n := signingkey.Create()
	n.SetKid(k.Kid)
	n.SetAlg(k.Alg)
	n.SetStatus(k.Status)
	n.SetPurpose(k.Purpose)
	n.SetPublicKey(k.PublicKey)
	n.SetPrivateKeyEnc(k.PrivateKeyEnc)
	n.SetNotBefore(k.NotBefore)
	n.SetNotAfter(k.NotAfter)
	_, err := n.Insert(ctx, s.ex(ctx))
	return database.MapErr(err)
}

// PromotePending and DemoteActive are addressed by PURPOSE and current status
// rather than by kid: the caller is rotating "the access key", and naming the
// kid would mean reading it first — a window in which a concurrent rotation
// could promote a different one.
func (s *Repository) PromotePending(ctx context.Context, purpose string) (int64, error) {
	n, err := authrquery.PromotePendingKey.Exec(ctx, s.ex(ctx), purpose)
	return n, database.MapErr(err)
}

func (s *Repository) DemoteActive(ctx context.Context, purpose string) (int64, error) {
	n, err := authrquery.DemoteActiveKey.Exec(ctx, s.ex(ctx), purpose)
	return n, database.MapErr(err)
}

func (s *Repository) SetKeyStatus(ctx context.Context, kid, status string) error {
	_, err := authrquery.SetSigningKeyStatus.Exec(ctx, s.ex(ctx), kid, status)
	return database.MapErr(err)
}

// ResealableKey is one signing key's sealed private half, with the kid its
// ciphertext is bound to.
type ResealableKey struct {
	ID     [16]byte
	Kid    string
	Sealed []byte
}

// ResealableKeys lists every signing key that still holds material.
//
// Retired keys included. Their rows survive so a retirement can be undone,
// and a row left sealed under a master that no longer exists cannot be.
func (s *Repository) ResealableKeys(ctx context.Context) ([]ResealableKey, error) {
	rows, err := signingkey.New().Order(signingkey.CreatedAt.Asc()).All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]ResealableKey, 0, len(rows))
	for _, r := range rows {
		if len(r.PrivateKeyEnc) == 0 {
			continue
		}
		out = append(out, ResealableKey{ID: r.ID, Kid: r.Kid, Sealed: r.PrivateKeyEnc})
	}
	return out, nil
}

// ResealKey writes private material rewrapped under a new master.
func (s *Repository) ResealKey(ctx context.Context, id [16]byte, sealed []byte) error {
	m := signingkey.MutateKey(id)
	m.SetPrivateKeyEnc(sealed)
	return database.MapErr(m.Update(ctx, s.ex(ctx)))
}
