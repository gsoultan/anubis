package repository

import (
	"context"
	"time"
)

type AuditReadRepository interface {
	QueryAudit(ctx context.Context, tenantID string, q AuditQuery) ([]AuditRecord, error)
	AuditChainRange(ctx context.Context, tenantID string, afterSeq int64, from, to *time.Time, batch int) ([]AuditRecord, error)
}
