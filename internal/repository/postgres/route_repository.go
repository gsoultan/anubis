package postgres

import (
	"context"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) ListRoutePolicies(ctx context.Context, applicationID string) ([]repository.RoutePolicyRecord, error) {
	rows, err := s.q(ctx).ListRoutePoliciesByApp(ctx, applicationID)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.RoutePolicyRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.RoutePolicyRecord{
			ID: r.ID, Priority: int(r.Priority), Effect: r.Effect,
			PathPattern: r.PathPattern, HostPattern: deref(r.HostPattern),
			Methods: r.Methods, PermissionKey: deref(r.PermissionKey),
			ScopeBindings: r.ScopeBindings,
		})
	}
	return out, nil
}

func (s *Store) ReplaceRoutePolicies(ctx context.Context, tenantID, applicationID string, policies []repository.RoutePolicyInput) error {
	return s.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.q(ctx).DeleteRoutePoliciesByApp(ctx, applicationID); err != nil {
			return mapErr(err)
		}
		for _, p := range policies {
			if err := s.q(ctx).InsertRoutePolicy(ctx, gen.InsertRoutePolicyParams{
				ApplicationID: applicationID, TenantID: tenantID,
				PermissionID: optStr(p.PermissionID), Priority: int32(p.Priority),
				Effect: p.Effect, PathPattern: p.PathPattern,
				HostPattern: p.HostPattern, Methods: p.Methods,
				ScopeBindings: orEmptyJSON(p.ScopeBindings),
			}); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
}
