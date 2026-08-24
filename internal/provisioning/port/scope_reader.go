package provisioningport

import (
	"context"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

// ScopeReader resolves a scope node's external reference — the id an
// operator knows it by — into the node itself.
type ScopeReader interface {
	ScopeNodeByRef(ctx context.Context, tenantID, axis, ref string) (*scopedomain.ScopeNodeRecord, error)
}
