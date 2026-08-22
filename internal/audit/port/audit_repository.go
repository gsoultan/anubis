package auditport

import (
	"context"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
)

// AuditRepository appends to the per-tenant hash chain. LockAuditChain must
// be called inside the same transaction as InsertAudit.
type AuditRepository interface {
	LockAuditChain(ctx context.Context, tenantID string) error
	LastAuditEntry(ctx context.Context, tenantID string) (seq int64, entryHash []byte, err error)
	InsertAudit(ctx context.Context, ev auditdomain.AuditEvent, seq int64, prevHash, entryHash []byte) (occurredAt time.Time, err error)
}
