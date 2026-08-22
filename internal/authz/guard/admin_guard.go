package guard

import (
	"context"
	"encoding/json"

	authzport "github.com/gsoultan/anubis/internal/authz/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// adminGuard gates admin operations through authorize() itself — Anubis is
// its own relying party. The caller's ACTIVE SCOPES travel as targets, which
// is what makes delegated administration work: a partner admin's grant is
// constrained to their partner_org node, so the same permission check that
// lets IT administer everyone lets them administer only their own company.
type Guard struct {
	authz authzport.AuthzRepository
}

func New(authz authzport.AuthzRepository) *Guard {
	return &Guard{authz: authz}
}

// Require answers who is calling after proving they hold the permission.
func (g *Guard) Require(ctx context.Context, permission string) (*authctx.Principal, error) {
	p, ok := authctx.From(ctx)
	if !ok {
		return nil, apperr.ErrUnauthenticated
	}
	targets, err := json.Marshal(p.Scopes)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	allow, err := g.authz.Authorize(ctx, p.IdentityID, p.TenantID, permission, targets)
	if err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	if !allow {
		return nil, apperr.ErrPermissionDenied.With("permission", permission)
	}
	return p, nil
}
