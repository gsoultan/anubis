package usecase

import (
	"context"
	"encoding/json"

	"github.com/gsoultan/anubis/internal/authctx"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

// adminGuard gates admin operations through authorize() itself — Anubis is
// its own relying party. The caller's ACTIVE SCOPES travel as targets, which
// is what makes delegated administration work: a partner admin's grant is
// constrained to their partner_org node, so the same permission check that
// lets IT administer everyone lets them administer only their own company.
type adminGuard struct {
	authz repository.AuthzRepository
}

func newAdminGuard(authz repository.AuthzRepository) *adminGuard {
	return &adminGuard{authz: authz}
}

// require answers who is calling after proving they hold the permission.
func (g *adminGuard) require(ctx context.Context, permission string) (*authctx.Principal, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, domain.ErrUnauthenticated
	}
	targets, err := json.Marshal(p.Scopes)
	if err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}
	allow, err := g.authz.Authorize(ctx, p.IdentityID, p.TenantID, permission, targets)
	if err != nil {
		return nil, domain.ErrInternal.Wrap(err)
	}
	if !allow {
		return nil, domain.ErrPermissionDenied.With("permission", permission)
	}
	return p, nil
}
