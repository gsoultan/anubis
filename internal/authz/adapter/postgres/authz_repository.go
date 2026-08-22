package authzpg

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/authz/adapter/postgres/gen"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) Authorize(ctx context.Context, identityID, tenantID, permission string, targets []byte) (bool, error) {
	allow, err := s.q(ctx).Authorize(ctx, gen.AuthorizeParams{
		IdentityID: identityID, TenantID: tenantID,
		Permission: permission, Targets: database.OrEmptyJSON(targets),
	})
	return allow, database.MapErr(err)
}

func (s *Repository) AuthorizeExplain(ctx context.Context, identityID, tenantID, permission string, targets []byte) (string, error) {
	detail, err := s.q(ctx).AuthorizeExplain(ctx, gen.AuthorizeExplainParams{
		IdentityID: identityID, TenantID: tenantID,
		Permission: permission, Targets: database.OrEmptyJSON(targets),
	})
	return detail, database.MapErr(err)
}

func (s *Repository) AuthorizeStrictSim(ctx context.Context, identityID, tenantID, permission string, targets []byte, strictAxis string) (bool, error) {
	allow, err := s.q(ctx).AuthorizeStrictSim(ctx, gen.AuthorizeStrictSimParams{
		IdentityID: identityID, TenantID: tenantID, Permission: database.OptStr(permission),
		Targets: database.OrEmptyJSON(targets), StrictAxis: strictAxis,
	})
	return allow, database.MapErr(err)
}

func (s *Repository) PermissionByKey(ctx context.Context, tenantID, key string) (*authzdomain.PermissionMeta, error) {
	row, err := s.q(ctx).GetPermissionByKey(ctx, gen.GetPermissionByKeyParams{
		TenantID: tenantID, Key: database.OptStr(key),
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	meta := &authzdomain.PermissionMeta{
		ID: row.ID, Key: database.Deref(row.Key), Risk: row.Risk,
		MinAssurance: int(row.MinAssurance), RequiresAMR: row.RequiresAmr,
		Deprecated: row.DeprecatedAt != nil,
	}
	if row.MaxAuthAge != "" {
		if d, perr := parsePgInterval(row.MaxAuthAge); perr == nil {
			meta.MaxAuthAge = d
		}
	}
	return meta, nil
}

func (s *Repository) RolesForIdentity(ctx context.Context, tenantID, identityID string) ([]string, error) {
	roles, err := s.q(ctx).RolesForIdentity(ctx, gen.RolesForIdentityParams{
		IdentityID: identityID, TenantID: tenantID,
	})
	return roles, database.MapErr(err)
}

func (s *Repository) EffectivePermissionsForIdentity(ctx context.Context, tenantID, identityID string) ([]string, error) {
	keys, err := s.q(ctx).EffectivePermissionsForIdentity(ctx, gen.EffectivePermissionsForIdentityParams{
		IdentityID: identityID, TenantID: tenantID,
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != nil {
			out = append(out, *k)
		}
	}
	return out, nil
}

func (s *Repository) SampleAuthorizeDecisions(ctx context.Context, tenantID string, n int) ([][]byte, error) {
	rows, err := s.q(ctx).SampleAuthorizeDecisions(ctx, gen.SampleAuthorizeDecisionsParams{
		TenantID: tenantID, SampleSize: int32(n),
	})
	return rows, database.MapErr(err)
}

// parsePgInterval understands the interval::text renderings the queries emit
// ("00:05:00", "2 days 01:00:00"). Postgres text intervals for auth-age use
// are hours/minutes/seconds and optional days.
func parsePgInterval(s string) (time.Duration, error) {
	var days int
	var hh, mm int
	var sec float64
	n, _ := fmtSscanf(s, "%d days %d:%d:%f", &days, &hh, &mm, &sec)
	if n < 4 {
		n, _ = fmtSscanf(s, "%d day %d:%d:%f", &days, &hh, &mm, &sec)
	}
	if n < 4 {
		days = 0
		if n2, err := fmtSscanf(s, "%d:%d:%f", &hh, &mm, &sec); n2 < 3 {
			return 0, errInvalidInterval(err)
		}
	}
	return time.Duration(days)*24*time.Hour +
		time.Duration(hh)*time.Hour +
		time.Duration(mm)*time.Minute +
		time.Duration(sec*float64(time.Second)), nil
}

func errInvalidInterval(err error) error {
	if err != nil {
		return err
	}
	return apperr.ErrInternal
}
