package controlpg

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/gsoultan/anubis/internal/control/adapter/postgres/gen"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// CreatePlatformUser adds an operator. The password arrives already hashed:
// this layer never sees a plaintext secret.
func (s *Repository) CreatePlatformUser(ctx context.Context, username, email, passwordHash string) (string, error) {
	arg := gen.CreatePlatformUserParams{Username: username, PasswordHash: passwordHash}
	if email != "" {
		arg.Email = &email
	}
	id, err := s.q(ctx).CreatePlatformUser(ctx, arg)
	if err != nil {
		return "", database.MapErr(err)
	}
	return id, nil
}

// PlatformUserByUsername is the sign-in lookup. Usernames are globally unique
// here, which is what lets console sign-in ask for one without a tenant.
func (s *Repository) PlatformUserByUsername(ctx context.Context, username string) (*controldomain.PlatformUser, string, error) {
	row, err := s.q(ctx).GetPlatformUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", nil
		}
		return nil, "", database.MapErr(err)
	}
	u := &controldomain.PlatformUser{
		ID: row.ID, Username: row.Username, Status: row.Status,
		TokenEpoch: int(row.TokenEpoch), CreatedAt: row.CreatedAt,
		LastLoginAt: row.LastLoginAt, DisabledAt: row.DisabledAt,
		TOTPEnrolledAt: row.TotpEnrolledAt, TOTPLastStep: uint64(row.TotpLastStep),
	}
	if row.Email != nil {
		u.Email = *row.Email
	}
	return u, row.PasswordHash, nil
}

// ListPlatformUsers returns one keyset page, ordered by username.
//
// The cursor is the last username on the page, not an offset: these tables
// grow, and OFFSET re-scans everything it skips and can show a row twice when
// one is inserted between requests.
func (s *Repository) ListPlatformUsers(ctx context.Context, query, after string, pageSize int32) ([]controldomain.PlatformUser, error) {
	rows, err := s.q(ctx).ListPlatformUsers(ctx, gen.ListPlatformUsersParams{
		Query: query, After: after, PageSize: pageSize,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]controldomain.PlatformUser, 0, len(rows))
	for _, r := range rows {
		u := controldomain.PlatformUser{
			ID: r.ID, Username: r.Username, Status: r.Status,
			TokenEpoch: int(r.TokenEpoch), CreatedAt: r.CreatedAt,
			LastLoginAt: r.LastLoginAt, DisabledAt: r.DisabledAt,
			TOTPEnrolledAt: r.TotpEnrolledAt,
		}
		if r.Email != nil {
			u.Email = *r.Email
		}
		out = append(out, u)
	}
	return out, nil
}

// CountPlatformUsers is the population behind a page, so a screen can say
// "50 of 4,812" rather than implying the page is all there is.
func (s *Repository) CountPlatformUsers(ctx context.Context) (int, error) {
	n, err := s.q(ctx).CountPlatformUsers(ctx)
	if err != nil {
		return 0, database.MapErr(err)
	}
	return int(n), nil
}

// AnyPlatformUser reports whether this installation has been set up at all.
func (s *Repository) AnyPlatformUser(ctx context.Context) (bool, error) {
	n, err := s.CountPlatformUsers(ctx)
	return n > 0, err
}

// TouchLogin records a successful sign-in.
func (s *Repository) TouchLogin(ctx context.Context, id string) {
	_ = s.q(ctx).TouchPlatformUserLogin(ctx, id)
}

// SetStatus disables or restores an operator. Disabling is immediate: the
// guard's per-request assignment lookup joins platform_users and requires an
// active account, so live tokens stop working rather than lingering until
// they expire.
func (s *Repository) SetStatus(ctx context.Context, id, status string) error {
	n, err := s.q(ctx).SetPlatformUserStatus(ctx, gen.SetPlatformUserStatusParams{ID: id, Status: status})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound.With("operator", id)
	}
	return nil
}

// TOTPSecret opens the sealed secret for one operator. It is unsealed only at
// the moment a code is checked, never held anywhere.
func (s *Repository) TOTPSecret(ctx context.Context, master []byte, id string) ([]byte, error) {
	row, err := s.q(ctx).GetPlatformUser(ctx, id)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if len(row.TotpSecretEnc) == 0 {
		return nil, nil
	}
	// The row id is the additional data, binding this ciphertext to this
	// account: a secret lifted into another row will not open.
	return keyring.OpenSecret(master, id, row.TotpSecretEnc)
}

// StageTOTPSecret stores an unconfirmed secret.
func (s *Repository) StageTOTPSecret(ctx context.Context, master []byte, id string, secret []byte) error {
	sealed, err := keyring.SealSecret(master, id, secret)
	if err != nil {
		return err
	}
	if _, err := s.q(ctx).StageTotpSecret(ctx, gen.StageTotpSecretParams{ID: id, Secret: sealed}); err != nil {
		return database.MapErr(err)
	}
	return nil
}

// ConfirmTOTP completes enrolment.
func (s *Repository) ConfirmTOTP(ctx context.Context, id string, step uint64) error {
	n, err := s.q(ctx).ConfirmTotpEnrolment(ctx, gen.ConfirmTotpEnrolmentParams{ID: id, Step: int64(step)})
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrInvalidArgument.With("totp", "no enrolment in progress")
	}
	return nil
}

// AdvanceTOTPStep enforces single use: it succeeds only when the step is
// strictly newer than the last accepted one, so a replayed code changes
// nothing and the caller refuses the sign-in.
func (s *Repository) AdvanceTOTPStep(ctx context.Context, id string, step uint64) (bool, error) {
	n, err := s.q(ctx).AdvanceTotpStep(ctx, gen.AdvanceTotpStepParams{ID: id, Step: int64(step)})
	if err != nil {
		return false, database.MapErr(err)
	}
	return n > 0, nil
}

// ClearTOTP removes a second factor.
func (s *Repository) ClearTOTP(ctx context.Context, id string) error {
	if _, err := s.q(ctx).ClearTotp(ctx, id); err != nil {
		return database.MapErr(err)
	}
	return nil
}

// PlatformUserByID is the lookup a challenge resolves against.
func (s *Repository) PlatformUserByID(ctx context.Context, id string) (*controldomain.PlatformUser, string, error) {
	row, err := s.q(ctx).GetPlatformUser(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", nil
		}
		return nil, "", database.MapErr(err)
	}
	u := &controldomain.PlatformUser{
		ID: row.ID, Username: row.Username, Status: row.Status,
		TokenEpoch: int(row.TokenEpoch), CreatedAt: row.CreatedAt,
		LastLoginAt: row.LastLoginAt, DisabledAt: row.DisabledAt,
		TOTPEnrolledAt: row.TotpEnrolledAt, TOTPLastStep: uint64(row.TotpLastStep),
	}
	if row.Email != nil {
		u.Email = *row.Email
	}
	return u, row.PasswordHash, nil
}
