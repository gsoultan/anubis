package authpg

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/auth/adapter/postgres/gen"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) CreateSession(ctx context.Context, in authdomain.SessionInput) (*authdomain.Session, error) {
	row, err := s.q(ctx).CreateSession(ctx, gen.CreateSessionParams{
		IdentityID:    in.IdentityID,
		TenantID:      in.TenantID,
		ApplicationID: database.OptStr(in.ApplicationID),
		Amr:           in.AMR,
		DeviceFp:      in.DeviceFP,
		Ip:            in.IP,
		UserAgent:     in.UserAgent,
		ActiveScopes:  database.OrEmptyJSON(in.ActiveScopes),
		ExpiresAt:     in.ExpiresAt,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &authdomain.Session{
		ID: row.ID, IdentityID: in.IdentityID, TenantID: in.TenantID,
		AMR: in.AMR, AuthTime: row.AuthTime, ExpiresAt: row.ExpiresAt,
	}, nil
}

func (s *Repository) SessionLive(ctx context.Context, id string) (*authdomain.SessionView, error) {
	row, err := s.q(ctx).GetSessionLive(ctx, id)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &authdomain.SessionView{
		ID: row.ID, IdentityID: row.IdentityID, TenantID: row.TenantID,
		ApplicationID: database.Deref(row.ApplicationID), AMR: row.Amr,
		CreatedAt: row.CreatedAt, LastSeenAt: row.LastSeenAt,
		AuthTime: row.AuthTime, ExpiresAt: row.ExpiresAt,
		ActiveScopes: row.ActiveScopes, DeviceFP: database.Deref(row.DeviceFp),
		TokenEpoch: int(row.TokenEpoch), IdentityStatus: row.IdentityStatus,
		AssuranceLevel: int(row.AssuranceLevel), Username: row.Username,
		Email: database.Deref(row.Email), RealmID: database.Deref(row.RealmID), RealmCode: database.Deref(row.RealmCode),
	}, nil
}

func (s *Repository) SessionsByIdentity(ctx context.Context, identityID string) ([]authdomain.SessionInfo, error) {
	rows, err := s.q(ctx).ListSessionsByIdentity(ctx, identityID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authdomain.SessionInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, authdomain.SessionInfo{
			ID: r.ID, CreatedAt: r.CreatedAt, LastSeenAt: r.LastSeenAt,
			ExpiresAt: r.ExpiresAt, AMR: r.Amr, IP: r.Ip, UserAgent: database.Deref(r.UserAgent),
		})
	}
	return out, nil
}

func (s *Repository) RevokeSession(ctx context.Context, tenantID, id, reason string) (*authdomain.RevokedSession, error) {
	row, err := s.q(ctx).RevokeSession(ctx, gen.RevokeSessionParams{
		ID: id, TenantID: tenantID, Reason: database.OptStr(reason),
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &authdomain.RevokedSession{
		ID: row.ID, IdentityID: row.IdentityID, ApplicationID: database.Deref(row.ApplicationID),
	}, nil
}

func (s *Repository) RevokeAllSessions(ctx context.Context, tenantID, identityID, reason string) ([]authdomain.RevokedSession, error) {
	rows, err := s.q(ctx).RevokeAllSessions(ctx, gen.RevokeAllSessionsParams{
		IdentityID: identityID, TenantID: tenantID, Reason: database.OptStr(reason),
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authdomain.RevokedSession, 0, len(rows))
	for _, r := range rows {
		out = append(out, authdomain.RevokedSession{
			ID: r.ID, IdentityID: identityID, ApplicationID: database.Deref(r.ApplicationID),
		})
	}
	return out, nil
}

func (s *Repository) TouchSession(ctx context.Context, id string) {
	_ = s.q(ctx).TouchSession(ctx, id)
}

func (s *Repository) UpdateSessionScopes(ctx context.Context, id string, scopes []byte) error {
	return database.MapErr(s.q(ctx).UpdateSessionScopes(ctx, gen.UpdateSessionScopesParams{
		ID: id, ActiveScopes: database.OrEmptyJSON(scopes),
	}))
}

func (s *Repository) UpgradeSessionAMR(ctx context.Context, id string, amr []string) (time.Time, error) {
	t, err := s.q(ctx).UpgradeSessionAmr(ctx, gen.UpgradeSessionAmrParams{ID: id, Amr: amr})
	return t, database.MapErr(err)
}

func (s *Repository) SetSessionCookieHash(ctx context.Context, id string, hash []byte) error {
	return database.MapErr(s.q(ctx).SetSessionCookieHash(ctx, gen.SetSessionCookieHashParams{
		ID: id, CookieHash: hash,
	}))
}

func (s *Repository) SessionByCookieHash(ctx context.Context, hash []byte) (*authdomain.SessionView, error) {
	row, err := s.q(ctx).GetSessionByCookieHash(ctx, hash)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return &authdomain.SessionView{
		ID: row.ID, IdentityID: row.IdentityID, TenantID: row.TenantID,
		AMR: row.Amr, AuthTime: row.AuthTime, ExpiresAt: row.ExpiresAt,
		ActiveScopes: row.ActiveScopes, Username: row.Username,
		RealmID: database.Deref(row.RealmID), RealmCode: database.Deref(row.RealmCode),
	}, nil
}

func (s *Repository) SessionState(ctx context.Context, tenantID, id string) (bool, bool, int, bool, error) {
	row, err := s.q(ctx).GetSessionState(ctx, gen.GetSessionStateParams{ID: id, TenantID: tenantID})
	if err != nil {
		return false, false, 0, false, database.MapErr(err)
	}
	revoked := row.RevokedAt != nil
	expired := !row.ExpiresAt.After(timeNow())
	blocked := row.IdentityStatus != "active" || row.DisabledAt != nil || row.AnonymizedAt != nil
	return revoked, expired, int(row.TokenEpoch), blocked, nil
}

// timeNow is a seam for the one place the adapter itself compares clocks.
var timeNow = time.Now
