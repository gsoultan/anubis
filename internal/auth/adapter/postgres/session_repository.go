package authpg

import (
	"context"
	"time"

	authrquery "github.com/gsoultan/anubis/internal/auth/adapter/postgres/rquery"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) CreateSession(ctx context.Context, in authdomain.SessionInput) (*authdomain.Session, error) {
	row, _, err := authrquery.CreateSession.One(ctx, s.ex(ctx),
		in.IdentityID, in.TenantID, optArg(in.ApplicationID), in.AMR,
		in.DeviceFP, in.IP, in.UserAgent,
		database.OrEmptyJSON(in.ActiveScopes), in.ExpiresAt)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &authdomain.Session{
		ID: row.ID, IdentityID: in.IdentityID, TenantID: in.TenantID,
		AMR: in.AMR, AuthTime: row.AuthTime, ExpiresAt: row.ExpiresAt,
	}, nil
}

// optArg turns an absent id into SQL NULL. ” would be a value the column's
// foreign key could never satisfy.
func optArg(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// SessionLive resolves a session AND the identity behind it in one query: the
// epoch and status are re-read on every call rather than trusted from when the
// session opened, so disabling somebody takes their live sessions with them.
func (s *Repository) SessionLive(ctx context.Context, id string) (*authdomain.SessionView, error) {
	row, ok, err := authrquery.GetSessionLive.One(ctx, s.ex(ctx), id)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	return &authdomain.SessionView{
		ID: row.ID, IdentityID: row.IdentityID, TenantID: row.TenantID,
		ApplicationID: nstr(row.ApplicationID), AMR: row.Amr,
		CreatedAt: row.CreatedAt, LastSeenAt: row.LastSeenAt,
		AuthTime: row.AuthTime, ExpiresAt: row.ExpiresAt,
		ActiveScopes: []byte(row.ActiveScopes), DeviceFP: nstr(row.DeviceFp),
		TokenEpoch: int(row.TokenEpoch), IdentityStatus: row.IdentityStatus,
		AssuranceLevel: int(row.AssuranceLevel), Username: nstr(row.Username),
		Email: nstr(row.Email), RealmID: nstr(row.RealmID), RealmCode: nstr(row.RealmCode),
	}, nil
}

func (s *Repository) SessionsByIdentity(ctx context.Context, identityID string) ([]authdomain.SessionInfo, error) {
	rows, err := authrquery.ListSessionsByIdentity.Query(ctx, s.ex(ctx), identityID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authdomain.SessionInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, authdomain.SessionInfo{
			ID: r.ID, CreatedAt: r.CreatedAt, LastSeenAt: r.LastSeenAt,
			ExpiresAt: r.ExpiresAt, AMR: r.Amr, IP: r.IP,
			UserAgent: nstr(r.UserAgent),
		})
	}
	return out, nil
}

// RevokeSession ends one session and clears its cookie hash in the same
// statement.
//
// A session that was already revoked is an ERROR, not a nil result. Every
// caller reads `err == nil` as "it was really revoked, now do the follow-up" —
// revoke its refresh tokens, check that it belonged to the caller — and the
// logout path dereferences the returned row to make that ownership check.
// Returning (nil, nil) made an already-revoked session panic there instead,
// which skipped the ownership check on the way past.
func (s *Repository) RevokeSession(ctx context.Context, tenantID, id, reason string) (*authdomain.RevokedSession, error) {
	row, ok, err := authrquery.RevokeSession.One(ctx, s.ex(ctx), id, tenantID, reason)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	return &authdomain.RevokedSession{
		ID: row.ID, IdentityID: row.IdentityID,
		ApplicationID: nstr(row.ApplicationID),
	}, nil
}

func (s *Repository) RevokeAllSessions(ctx context.Context, tenantID, identityID, reason string) ([]authdomain.RevokedSession, error) {
	rows, err := authrquery.RevokeAllSessions.Query(ctx, s.ex(ctx), identityID, tenantID, reason)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authdomain.RevokedSession, 0, len(rows))
	for _, r := range rows {
		out = append(out, authdomain.RevokedSession{
			ID: r.ID, IdentityID: identityID,
			ApplicationID: nstr(r.ApplicationID),
		})
	}
	return out, nil
}

// TouchSession is best effort: a request that authenticated must not fail
// because its activity timestamp did not land.
func (s *Repository) TouchSession(ctx context.Context, id string) {
	_, _ = authrquery.TouchSession.Exec(ctx, s.ex(ctx), id)
}

func (s *Repository) UpdateSessionScopes(ctx context.Context, id string, scopes []byte) error {
	_, err := authrquery.UpdateSessionScopes.Exec(ctx, s.ex(ctx), id, database.OrEmptyJSON(scopes))
	return database.MapErr(err)
}

// UpgradeSessionAMR is the step-up. auth_time restarts, which is what a
// max_age check reads.
func (s *Repository) UpgradeSessionAMR(ctx context.Context, id string, amr []string) (time.Time, error) {
	row, ok, err := authrquery.UpgradeSessionAmr.One(ctx, s.ex(ctx), id, amr)
	if err != nil {
		return time.Time{}, database.MapErr(err)
	}
	if !ok {
		return time.Time{}, database.NotFound()
	}
	return row.AuthTime, nil
}

func (s *Repository) SetSessionCookieHash(ctx context.Context, id string, hash []byte) error {
	_, err := authrquery.SetSessionCookieHash.Exec(ctx, s.ex(ctx), id, hash)
	return database.MapErr(err)
}

func (s *Repository) SessionByCookieHash(ctx context.Context, hash []byte) (*authdomain.SessionView, error) {
	row, ok, err := authrquery.GetSessionByCookieHash.One(ctx, s.ex(ctx), hash)
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, database.NotFound()
	}
	return &authdomain.SessionView{
		ID: row.ID, IdentityID: row.IdentityID, TenantID: row.TenantID,
		AMR: row.Amr, AuthTime: row.AuthTime, ExpiresAt: row.ExpiresAt,
		ActiveScopes: []byte(row.ActiveScopes), Username: nstr(row.Username),
		RealmID: nstr(row.RealmID), RealmCode: nstr(row.RealmCode),
	}, nil
}

// SessionState answers introspection: revoked, expired, the identity's epoch,
// and whether the identity is blocked.
func (s *Repository) SessionState(ctx context.Context, tenantID, id string) (bool, bool, int, bool, error) {
	row, ok, err := authrquery.GetSessionState.One(ctx, s.ex(ctx), id, tenantID)
	if err != nil {
		return false, false, 0, false, database.MapErr(err)
	}
	if !ok {
		return false, false, 0, false, database.NotFound()
	}
	_, revoked := row.RevokedAt.Get()
	expired := !row.ExpiresAt.After(timeNow())
	_, disabled := row.DisabledAt.Get()
	_, anonymized := row.AnonymizedAt.Get()
	blocked := row.IdentityStatus != "active" || disabled || anonymized
	return revoked, expired, int(row.TokenEpoch), blocked, nil
}

// timeNow is a seam for the one place the adapter itself compares clocks.
var timeNow = time.Now
