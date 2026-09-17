package tenancypg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/rgen/routepolicy"
	tenancyrquery "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/rquery"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	"github.com/gsoultan/storm/runtime"
)

func (s *Repository) ListRoutePolicies(ctx context.Context, applicationID string) ([]tenancydomain.RoutePolicyRecord, error) {
	rows, err := tenancyrquery.ListRoutePoliciesByApp.Query(ctx, s.ex(ctx), applicationID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]tenancydomain.RoutePolicyRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, tenancydomain.RoutePolicyRecord{
			ID: r.ID, Priority: int(r.Priority), Effect: r.Effect,
			PathPattern: r.PathPattern, HostPattern: nstr(r.HostPattern),
			Methods: r.Methods, PermissionKey: nstr(r.PermissionKey),
			ScopeBindings: []byte(r.ScopeBindings),
		})
	}
	return out, nil
}

// ReplaceRoutePolicies swaps an application's whole rule set.
//
// Delete-then-insert inside ONE transaction, because the rules are evaluated
// in priority order and a partial set is a different policy: a request
// arriving between the delete and the last insert would be matched against
// whatever had landed so far.
func (s *Repository) ReplaceRoutePolicies(ctx context.Context, tenantID, applicationID string, policies []tenancydomain.RoutePolicyInput) error {
	appID, err := database.ParseUUID(applicationID)
	if err != nil {
		return database.MapErr(err)
	}
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := tenancyrquery.DeleteRoutePoliciesByApp.Exec(ctx, s.ex(ctx), applicationID); err != nil {
			return database.MapErr(err)
		}
		for _, p := range policies {
			n := routepolicy.Create()
			n.SetApplicationID(appID)
			n.SetTenantID(tid)
			n.SetPriority(int32(p.Priority))
			n.SetEffect(p.Effect)
			n.SetPathPattern(p.PathPattern)
			// Only when the caller named some. The column defaults to
			// '{*}' — every method — and a masked insert that assigns nil
			// writes NULL over that default rather than leaving it, which
			// the NOT NULL rejects.
			if len(p.Methods) > 0 {
				n.SetMethods(p.Methods)
			}
			n.SetScopeBindings(runtime.JSON(database.OrEmptyJSON(p.ScopeBindings)))
			if p.PermissionID == "" {
				n.SetPermissionIDNull()
			} else {
				pid, err := database.ParseUUID(p.PermissionID)
				if err != nil {
					return database.MapErr(err)
				}
				n.SetPermissionID(pid)
			}
			// An empty host pattern means "any host", which is NULL here —
			// '' would be a pattern that matches nothing.
			if p.HostPattern == "" {
				n.SetHostPatternNull()
			} else {
				n.SetHostPattern(p.HostPattern)
			}
			if _, err := n.Insert(ctx, s.ex(ctx)); err != nil {
				return database.MapErr(err)
			}
		}
		return nil
	})
}
