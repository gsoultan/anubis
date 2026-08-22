package postgres

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) Authorize(ctx context.Context, identityID, tenantID, permission string, targets []byte) (bool, error) {
	allow, err := s.q(ctx).Authorize(ctx, gen.AuthorizeParams{
		IdentityID: identityID, TenantID: tenantID,
		Permission: permission, Targets: orEmptyJSON(targets),
	})
	return allow, mapErr(err)
}

func (s *Store) AuthorizeExplain(ctx context.Context, identityID, tenantID, permission string, targets []byte) (string, error) {
	detail, err := s.q(ctx).AuthorizeExplain(ctx, gen.AuthorizeExplainParams{
		IdentityID: identityID, TenantID: tenantID,
		Permission: permission, Targets: orEmptyJSON(targets),
	})
	return detail, mapErr(err)
}

func (s *Store) AuthorizeStrictSim(ctx context.Context, identityID, tenantID, permission string, targets []byte, strictAxis string) (bool, error) {
	allow, err := s.q(ctx).AuthorizeStrictSim(ctx, gen.AuthorizeStrictSimParams{
		IdentityID: identityID, TenantID: tenantID, Permission: optStr(permission),
		Targets: orEmptyJSON(targets), StrictAxis: strictAxis,
	})
	return allow, mapErr(err)
}

func (s *Store) PermissionByKey(ctx context.Context, tenantID, key string) (*repository.PermissionMeta, error) {
	row, err := s.q(ctx).GetPermissionByKey(ctx, gen.GetPermissionByKeyParams{
		TenantID: tenantID, Key: optStr(key),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	meta := &repository.PermissionMeta{
		ID: row.ID, Key: deref(row.Key), Risk: row.Risk,
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

func (s *Store) RolesForIdentity(ctx context.Context, tenantID, identityID string) ([]string, error) {
	roles, err := s.q(ctx).RolesForIdentity(ctx, gen.RolesForIdentityParams{
		IdentityID: identityID, TenantID: tenantID,
	})
	return roles, mapErr(err)
}

func (s *Store) EffectivePermissionsForIdentity(ctx context.Context, tenantID, identityID string) ([]string, error) {
	keys, err := s.q(ctx).EffectivePermissionsForIdentity(ctx, gen.EffectivePermissionsForIdentityParams{
		IdentityID: identityID, TenantID: tenantID,
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != nil {
			out = append(out, *k)
		}
	}
	return out, nil
}

func (s *Store) SampleAuthorizeDecisions(ctx context.Context, tenantID string, n int) ([][]byte, error) {
	rows, err := s.q(ctx).SampleAuthorizeDecisions(ctx, gen.SampleAuthorizeDecisionsParams{
		TenantID: tenantID, SampleSize: int32(n),
	})
	return rows, mapErr(err)
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
	return domain.ErrInternal
}
