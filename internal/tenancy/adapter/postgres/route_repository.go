package tenancypg

import (
	"context"

	"github.com/gsoultan/anubis/internal/platform/database"
	gen "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/gen"
	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
)

func (s *Repository) ListRoutePolicies(ctx context.Context, applicationID string) ([]tenancydomain.RoutePolicyRecord, error) {
	rows, err := s.q(ctx).ListRoutePoliciesByApp(ctx, applicationID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]tenancydomain.RoutePolicyRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, tenancydomain.RoutePolicyRecord{
			ID: r.ID, Priority: int(r.Priority), Effect: r.Effect,
			PathPattern: r.PathPattern, HostPattern: database.Deref(r.HostPattern),
			Methods: r.Methods, PermissionKey: database.Deref(r.PermissionKey),
			ScopeBindings: r.ScopeBindings,
		})
	}
	return out, nil
}

func (s *Repository) ReplaceRoutePolicies(ctx context.Context, tenantID, applicationID string, policies []tenancydomain.RoutePolicyInput) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.q(ctx).DeleteRoutePoliciesByApp(ctx, applicationID); err != nil {
			return database.MapErr(err)
		}
		for _, p := range policies {
			if err := s.q(ctx).InsertRoutePolicy(ctx, gen.InsertRoutePolicyParams{
				ApplicationID: applicationID, TenantID: tenantID,
				PermissionID: database.OptStr(p.PermissionID), Priority: int32(p.Priority),
				Effect: p.Effect, PathPattern: p.PathPattern,
				HostPattern: p.HostPattern, Methods: p.Methods,
				ScopeBindings: database.OrEmptyJSON(p.ScopeBindings),
			}); err != nil {
				return database.MapErr(err)
			}
		}
		return nil
	})
}
