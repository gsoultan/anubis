package authzpg

import (
	"context"
	"time"

	authzrquery "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rquery"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) Authorize(ctx context.Context, identityID, tenantID, permission string, targets []byte) (bool, error) {
	row, ok, err := authzrquery.Authorize.One(ctx, s.rex(ctx),
		identityID, tenantID, permission, database.OrEmptyJSON(targets))
	if err != nil {
		return false, database.MapErr(err)
	}
	if !ok {
		// authorize() is scalar and always yields a row; no row means the
		// call never ran. Deny and say why rather than invent a decision.
		return false, apperr.ErrNotFound
	}
	return row.Allow, nil
}

func (s *Repository) AuthorizeExplain(ctx context.Context, identityID, tenantID, permission string, targets []byte) (string, error) {
	row, ok, err := authzrquery.AuthorizeExplain.One(ctx, s.rex(ctx),
		identityID, tenantID, permission, database.OrEmptyJSON(targets))
	if err != nil {
		return "", database.MapErr(err)
	}
	if !ok {
		return "", apperr.ErrNotFound
	}
	return row.Detail, nil
}

func (s *Repository) AuthorizeStrictSim(ctx context.Context, identityID, tenantID, permission string, targets []byte, strictAxis string) (bool, error) {
	row, ok, err := authzrquery.AuthorizeStrictSim.One(ctx, s.rex(ctx),
		identityID, tenantID, database.OptStr(permission),
		database.OrEmptyJSON(targets), strictAxis)
	if err != nil {
		return false, database.MapErr(err)
	}
	if !ok {
		return false, apperr.ErrNotFound
	}
	return row.Allow, nil
}

func (s *Repository) PermissionByKey(ctx context.Context, tenantID, key string) (*authzdomain.PermissionMeta, error) {
	row, ok, err := authzrquery.GetPermissionByKey.One(ctx, s.rex(ctx),
		tenantID, database.OptStr(key))
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		return nil, apperr.ErrNotFound
	}
	meta := &authzdomain.PermissionMeta{
		ID: row.ID, Key: row.Key.V, Risk: row.Risk,
		MinAssurance: int(row.MinAssurance), RequiresAMR: row.RequiresAmr,
		Deprecated: row.DeprecatedAt.Valid,
	}
	if row.MaxAuthAge != "" {
		if d, perr := parsePgInterval(row.MaxAuthAge); perr == nil {
			meta.MaxAuthAge = d
		}
	}
	return meta, nil
}

func (s *Repository) RolesForIdentity(ctx context.Context, tenantID, identityID string) ([]string, error) {
	rows, err := authzrquery.RolesForIdentity.Query(ctx, s.rex(ctx), identityID, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	// nil (not empty) when no rows, as the sqlc form returned.
	var roles []string
	for _, r := range rows {
		roles = append(roles, r.Name)
	}
	return roles, nil
}

func (s *Repository) EffectivePermissionsForIdentity(ctx context.Context, tenantID, identityID string) ([]string, error) {
	rows, err := authzrquery.EffectivePermissionsForIdentity.Query(ctx, s.rex(ctx), identityID, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Key.Valid {
			out = append(out, r.Key.V)
		}
	}
	return out, nil
}

func (s *Repository) SampleAuthorizeDecisions(ctx context.Context, tenantID string, n int) ([][]byte, error) {
	rows, err := authzrquery.SampleAuthorizeDecisions.Query(ctx, s.rex(ctx), tenantID, n)
	if err != nil {
		return nil, database.MapErr(err)
	}
	var out [][]byte
	for _, r := range rows {
		out = append(out, []byte(r.Detail))
	}
	return out, nil
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

// EffectiveGrantsForIdentity returns the identity's live grants with scopes.
//
// The query returns ONE ROW PER SCOPE, so a grant with three scopes arrives as
// three rows and an unscoped grant as one row with the scope columns null.
// Reassembling here rather than in SQL keeps the statement something a reader
// can check against the schema.
func (s *Repository) EffectiveGrantsForIdentity(ctx context.Context, tenantID, identityID string) ([]authzdomain.EffectiveGrant, error) {
	rows, err := authzrquery.EffectiveGrantsForIdentity.Query(ctx, s.rex(ctx), identityID, tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}

	// Ordered by the query, so a grant's rows are contiguous and the index
	// keeps insertion order rather than the map's.
	byID := make(map[string]int, len(rows))
	out := make([]authzdomain.EffectiveGrant, 0, len(rows))

	for _, r := range rows {
		i, seen := byID[r.GrantID]
		if !seen {
			out = append(out, authzdomain.EffectiveGrant{
				ID: r.GrantID, RoleID: r.RoleID, Role: r.RoleName,
				SelfScoped: r.SelfScoped, Permissions: r.Permissions,
			})
			i = len(out) - 1
			byID[r.GrantID] = i
		}
		// A null axis is the LEFT JOIN finding no scope row: the grant is
		// unscoped, and that is a grant with no scopes rather than a scope with
		// no axis.
		if !r.Axis.Valid || !r.NodeID.Valid {
			continue
		}
		out[i].Scopes = append(out[i].Scopes, authzdomain.GrantScope{
			Axis:    r.Axis.V,
			NodeID:  r.NodeID.V,
			Inherit: r.Inherit.Valid && r.Inherit.V,
			Exclude: r.Exclude.Valid && r.Exclude.V,
		})
	}
	return out, nil
}

// ScopeForestForTenant returns every live node on the requested axes.
func (s *Repository) ScopeForestForTenant(ctx context.Context, tenantID string, axes []string) ([]authzdomain.ForestNode, error) {
	rows, err := authzrquery.ScopeForestForTenant.Query(ctx, s.rex(ctx), tenantID, axes)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]authzdomain.ForestNode, 0, len(rows))
	for _, r := range rows {
		out = append(out, authzdomain.ForestNode{
			Axis: r.AxisCode, ID: r.ID,
			Parent: r.ParentID.V, ParentAxis: r.ParentAxis.V,
			Label: r.Name,
		})
	}
	return out, nil
}
