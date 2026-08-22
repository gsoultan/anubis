package postgres

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) CreateSession(ctx context.Context, in repository.SessionInput) (*domain.Session, error) {
	row, err := s.q(ctx).CreateSession(ctx, gen.CreateSessionParams{
		IdentityID:    in.IdentityID,
		TenantID:      in.TenantID,
		ApplicationID: optStr(in.ApplicationID),
		Amr:           in.AMR,
		DeviceFp:      in.DeviceFP,
		Ip:            in.IP,
		UserAgent:     in.UserAgent,
		ActiveScopes:  orEmptyJSON(in.ActiveScopes),
		ExpiresAt:     in.ExpiresAt,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return &domain.Session{
		ID: row.ID, IdentityID: in.IdentityID, TenantID: in.TenantID,
		AMR: in.AMR, AuthTime: row.AuthTime, ExpiresAt: row.ExpiresAt,
	}, nil
}

func (s *Store) SessionLive(ctx context.Context, id string) (*repository.SessionView, error) {
	row, err := s.q(ctx).GetSessionLive(ctx, id)
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.SessionView{
		ID: row.ID, IdentityID: row.IdentityID, TenantID: row.TenantID,
		ApplicationID: deref(row.ApplicationID), AMR: row.Amr,
		CreatedAt: row.CreatedAt, LastSeenAt: row.LastSeenAt,
		AuthTime: row.AuthTime, ExpiresAt: row.ExpiresAt,
		ActiveScopes: row.ActiveScopes, DeviceFP: deref(row.DeviceFp),
		TokenEpoch: int(row.TokenEpoch), IdentityStatus: row.IdentityStatus,
		AssuranceLevel: int(row.AssuranceLevel), Username: row.Username,
		Email: deref(row.Email), RealmID: deref(row.RealmID), RealmCode: deref(row.RealmCode),
	}, nil
}

func (s *Store) SessionsByIdentity(ctx context.Context, identityID string) ([]repository.SessionInfo, error) {
	rows, err := s.q(ctx).ListSessionsByIdentity(ctx, identityID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.SessionInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.SessionInfo{
			ID: r.ID, CreatedAt: r.CreatedAt, LastSeenAt: r.LastSeenAt,
			ExpiresAt: r.ExpiresAt, AMR: r.Amr, IP: r.Ip, UserAgent: deref(r.UserAgent),
		})
	}
	return out, nil
}

func (s *Store) RevokeSession(ctx context.Context, tenantID, id, reason string) (*repository.RevokedSession, error) {
	row, err := s.q(ctx).RevokeSession(ctx, gen.RevokeSessionParams{
		ID: id, TenantID: tenantID, Reason: optStr(reason),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.RevokedSession{
		ID: row.ID, IdentityID: row.IdentityID, ApplicationID: deref(row.ApplicationID),
	}, nil
}

func (s *Store) RevokeAllSessions(ctx context.Context, tenantID, identityID, reason string) ([]repository.RevokedSession, error) {
	rows, err := s.q(ctx).RevokeAllSessions(ctx, gen.RevokeAllSessionsParams{
		IdentityID: identityID, TenantID: tenantID, Reason: optStr(reason),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.RevokedSession, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.RevokedSession{
			ID: r.ID, IdentityID: identityID, ApplicationID: deref(r.ApplicationID),
		})
	}
	return out, nil
}

func (s *Store) TouchSession(ctx context.Context, id string) {
	_ = s.q(ctx).TouchSession(ctx, id)
}

func (s *Store) UpdateSessionScopes(ctx context.Context, id string, scopes []byte) error {
	return mapErr(s.q(ctx).UpdateSessionScopes(ctx, gen.UpdateSessionScopesParams{
		ID: id, ActiveScopes: orEmptyJSON(scopes),
	}))
}

func (s *Store) UpgradeSessionAMR(ctx context.Context, id string, amr []string) (time.Time, error) {
	t, err := s.q(ctx).UpgradeSessionAmr(ctx, gen.UpgradeSessionAmrParams{ID: id, Amr: amr})
	return t, mapErr(err)
}

func (s *Store) SetSessionCookieHash(ctx context.Context, id string, hash []byte) error {
	return mapErr(s.q(ctx).SetSessionCookieHash(ctx, gen.SetSessionCookieHashParams{
		ID: id, CookieHash: hash,
	}))
}

func (s *Store) SessionByCookieHash(ctx context.Context, hash []byte) (*repository.SessionView, error) {
	row, err := s.q(ctx).GetSessionByCookieHash(ctx, hash)
	if err != nil {
		return nil, mapErr(err)
	}
	return &repository.SessionView{
		ID: row.ID, IdentityID: row.IdentityID, TenantID: row.TenantID,
		AMR: row.Amr, AuthTime: row.AuthTime, ExpiresAt: row.ExpiresAt,
		ActiveScopes: row.ActiveScopes, Username: row.Username,
		RealmID: deref(row.RealmID), RealmCode: deref(row.RealmCode),
	}, nil
}

func (s *Store) SessionState(ctx context.Context, tenantID, id string) (bool, bool, int, bool, error) {
	row, err := s.q(ctx).GetSessionState(ctx, gen.GetSessionStateParams{ID: id, TenantID: tenantID})
	if err != nil {
		return false, false, 0, false, mapErr(err)
	}
	revoked := row.RevokedAt != nil
	expired := !row.ExpiresAt.After(timeNow())
	blocked := row.IdentityStatus != "active" || row.DisabledAt != nil || row.AnonymizedAt != nil
	return revoked, expired, int(row.TokenEpoch), blocked, nil
}

// timeNow is a seam for the one place the adapter itself compares clocks.
var timeNow = time.Now
