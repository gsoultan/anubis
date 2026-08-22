package auditport

import (
	"context"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
)

// Auditor appends to the hash chain. Implementations must serialise appends
// per tenant (the chain is per tenant) and never drop security events
// silently.
type Auditor interface {
	Emit(ctx context.Context, ev auditdomain.AuditEvent)
}
