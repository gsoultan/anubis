package auditport

import (
	"context"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
)

type AuditReadRepository interface {
	QueryAudit(ctx context.Context, tenantID string, q auditdomain.AuditQuery) ([]auditdomain.AuditRecord, error)
	AuditChainRange(ctx context.Context, tenantID string, afterSeq int64, from, to *time.Time, batch int) ([]auditdomain.AuditRecord, error)
}
