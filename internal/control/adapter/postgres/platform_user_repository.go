package controlpg

import (
	"context"

	"github.com/gsoultan/anubis/internal/control/adapter/postgres/rgen/platformuser"
	controlrquery "github.com/gsoultan/anubis/internal/control/adapter/postgres/rquery"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// CreatePlatformUser adds an operator. The password arrives already hashed:
// this layer never sees a plaintext secret.
func (s *Repository) CreatePlatformUser(ctx context.Context, username, email, passwordHash string) (string, error) {
	n := platformuser.Create()
	n.SetUsername(username)
	n.SetPasswordHash(passwordHash)
	if email == "" {
		// NULL, not '': the unique index on lower(email) is partial on
		// "email <> ''", so an empty string would be a value it does not
		// cover while NULL is the absence it was written for.
		n.SetEmailNull()
	} else {
		n.SetEmail(email)
	}
	row, err := n.Insert(ctx, s.ex(ctx))
	if err != nil {
		return "", database.MapErr(err)
	}
	return database.UUIDStr(row.ID), nil
}

// PlatformUserByUsername is the sign-in lookup. Usernames are globally unique
// here, which is what lets console sign-in ask for one without a tenant.
//
// The match is on lower(username), which is the expression the unique index is
// built on — comparing any other way would either miss a row or scan. EqLower
// is the predicate that says so; a plain Eq here is a sequential scan on every
// sign-in attempt, and it looks identical at the call site.
func (s *Repository) PlatformUserByUsername(ctx context.Context, username string) (*controldomain.PlatformUser, string, error) {
	row, ok, err := platformuser.New().
		Where(platformuser.Username.EqLower(username)).
		One(ctx, s.ex(ctx))
	if err != nil {
		return nil, "", database.MapErr(err)
	}
	if !ok {
		return nil, "", nil
	}
	return userOf(row), row.PasswordHash, nil
}

// PlatformUserByID is the lookup a challenge resolves against.
func (s *Repository) PlatformUserByID(ctx context.Context, id string) (*controldomain.PlatformUser, string, error) {
	uid, err := database.ParseUUID(id)
	if err != nil {
		return nil, "", database.MapErr(err)
	}
	row, ok, err := platformuser.New().Where(platformuser.ID.Eq(uid)).One(ctx, s.ex(ctx))
	if err != nil {
		return nil, "", database.MapErr(err)
	}
	if !ok {
		return nil, "", nil
	}
	return userOf(row), row.PasswordHash, nil
}

// userOf maps a row onto the domain type. The password hash is returned
// separately by the callers rather than carried on the struct, so a value that
// reaches a log or a response cannot contain it.
func userOf(row platformuser.Row) *controldomain.PlatformUser {
	u := &controldomain.PlatformUser{
		ID: database.UUIDStr(row.ID), Username: row.Username, Status: row.Status,
		TokenEpoch: int(row.TokenEpoch), CreatedAt: row.CreatedAt,
		LastLoginAt: tptr(row.LastLoginAt), DisabledAt: tptr(row.DisabledAt),
		TOTPEnrolledAt: tptr(row.TotpEnrolledAt), TOTPLastStep: uint64(row.TotpLastStep),
	}
	if e, ok := row.Email.Get(); ok {
		u.Email = e
	}
	return u
}

// AnyPlatformUser reports whether this installation has been set up at all.
func (s *Repository) AnyPlatformUser(ctx context.Context) (bool, error) {
	n, err := s.CountPlatformUsers(ctx)
	return n > 0, err
}

// ListPlatformUsers returns one keyset page, ordered by username.
//
// The cursor is the last username on the page, not an offset: these tables
// grow, and OFFSET re-scans everything it skips and can show a row twice when
// one is inserted between requests.
func (s *Repository) ListPlatformUsers(ctx context.Context, query, after string, pageSize int32) ([]controldomain.PlatformUser, error) {
	rows, err := controlrquery.ListPlatformUsers.Query(ctx, s.ex(ctx), query, after, pageSize)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]controldomain.PlatformUser, 0, len(rows))
	for _, r := range rows {
		u := controldomain.PlatformUser{
			ID: r.ID, Username: r.Username, Status: r.Status,
			TokenEpoch: int(r.TokenEpoch), CreatedAt: r.CreatedAt,
			LastLoginAt: tptr(r.LastLoginAt), DisabledAt: tptr(r.DisabledAt),
			TOTPEnrolledAt: tptr(r.TotpEnrolledAt),
		}
		if e, ok := r.Email.Get(); ok {
			u.Email = e
		}
		out = append(out, u)
	}
	return out, nil
}

// CountPlatformUsers is the population behind a page, so a screen can say
// "50 of 4,812" rather than implying the page is all there is.
func (s *Repository) CountPlatformUsers(ctx context.Context) (int, error) {
	n, err := platformuser.New().Count(ctx, s.ex(ctx))
	if err != nil {
		return 0, database.MapErr(err)
	}
	return int(n), nil
}

// TouchLogin records a successful sign-in. Best effort: a sign-in that worked
// must not fail because its timestamp did not land.
func (s *Repository) TouchLogin(ctx context.Context, id string) {
	_, _ = controlrquery.TouchPlatformUserLogin.Exec(ctx, s.ex(ctx), id)
}

// SetStatus disables or restores an operator. Disabling is immediate: the
// guard's per-request assignment lookup joins platform_users and requires an
// active account, so live tokens stop working rather than lingering until
// they expire.
func (s *Repository) SetStatus(ctx context.Context, id, status string) error {
	n, err := controlrquery.SetPlatformUserStatus.Exec(ctx, s.ex(ctx), id, status)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound.With("operator", id)
	}
	return nil
}

// RehashPassword replaces a hash with one at the current cost, and only if
// the stored hash is still the one that was just verified.
//
// Best effort by design: a login that verified must not fail because the
// upgrade lost a race. It returns nil when nothing matched.
func (s *Repository) RehashPassword(ctx context.Context, id, oldHash, newHash string) error {
	if _, err := controlrquery.RehashPlatformUserPassword.Exec(ctx, s.ex(ctx), id, oldHash, newHash); err != nil {
		return database.MapErr(err)
	}
	return nil
}

// SetPassword replaces the password and advances the token epoch, so every
// token minted under the old one stops working.
func (s *Repository) SetPassword(ctx context.Context, id, newHash string) error {
	n, err := controlrquery.SetPlatformUserPassword.Exec(ctx, s.ex(ctx), id, newHash)
	if err != nil {
		return database.MapErr(err)
	}
	if n == 0 {
		return apperr.ErrNotFound.With("operator", id)
	}
	return nil
}

// TOTPSecret opens the sealed secret for one operator. It is unsealed only at
// the moment a code is checked, never held.
func (s *Repository) TOTPSecret(ctx context.Context, master []byte, id string) ([]byte, error) {
	uid, err := database.ParseUUID(id)
	if err != nil {
		return nil, database.MapErr(err)
	}
	row, ok, err := platformuser.New().Where(platformuser.ID.Eq(uid)).One(ctx, s.ex(ctx))
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok || len(row.TotpSecretEnc) == 0 {
		return nil, nil
	}
	return keyring.OpenSecret(master, id, row.TotpSecretEnc)
}

// StageTOTPSecret stores an unconfirmed secret.
func (s *Repository) StageTOTPSecret(ctx context.Context, master []byte, id string, secret []byte) error {
	sealed, err := keyring.SealSecret(master, id, secret)
	if err != nil {
		return err
	}
	if _, err := controlrquery.StageTotpSecret.Exec(ctx, s.ex(ctx), id, sealed); err != nil {
		return database.MapErr(err)
	}
	return nil
}

// ConfirmTOTP completes enrolment.
func (s *Repository) ConfirmTOTP(ctx context.Context, id string, step uint64) error {
	n, err := controlrquery.ConfirmTotpEnrolment.Exec(ctx, s.ex(ctx), id, int64(step))
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
	n, err := controlrquery.AdvanceTotpStep.Exec(ctx, s.ex(ctx), id, int64(step))
	if err != nil {
		return false, database.MapErr(err)
	}
	return n > 0, nil
}

// ClearTOTP removes a second factor.
func (s *Repository) ClearTOTP(ctx context.Context, id string) error {
	if _, err := controlrquery.ClearTotp.Exec(ctx, s.ex(ctx), id); err != nil {
		return database.MapErr(err)
	}
	return nil
}

// ResealableTOTP is one operator's sealed TOTP secret, with the id its
// ciphertext is bound to.
type ResealableTOTP struct {
	ID     [16]byte
	IDText string
	Sealed []byte
}

// ResealableTOTPSecrets lists every operator holding sealed TOTP material.
func (s *Repository) ResealableTOTPSecrets(ctx context.Context) ([]ResealableTOTP, error) {
	rows, err := platformuser.New().Order(platformuser.CreatedAt.Asc()).All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]ResealableTOTP, 0, len(rows))
	for _, r := range rows {
		if len(r.TotpSecretEnc) == 0 {
			continue
		}
		out = append(out, ResealableTOTP{
			ID: r.ID, IDText: database.UUIDStr(r.ID), Sealed: r.TotpSecretEnc,
		})
	}
	return out, nil
}

// ResealTOTPSecret writes a secret rewrapped under a new master.
func (s *Repository) ResealTOTPSecret(ctx context.Context, id [16]byte, sealed []byte) error {
	m := platformuser.MutateKey(id)
	m.SetTotpSecretEnc(sealed)
	return database.MapErr(m.Update(ctx, s.ex(ctx)))
}
