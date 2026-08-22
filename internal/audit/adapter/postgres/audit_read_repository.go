package auditpg

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/audit/adapter/postgres/gen"
	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

func (s *Repository) QueryAudit(ctx context.Context, tenantID string, q auditdomain.AuditQuery) ([]auditdomain.AuditRecord, error) {
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var beforeSeq *int64 = q.BeforeSeq
	rows, err := s.q(ctx).QueryAudit(ctx, gen.QueryAuditParams{
		TenantID:  tenantID,
		ActorID:   database.OptStr(q.ActorID),
		Action:    database.OptStr(q.Action),
		FromTs:    q.From,
		ToTs:      q.To,
		BeforeSeq: beforeSeq,
		PageSize:  int32(limit),
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]auditdomain.AuditRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, auditdomain.AuditRecord{
			ID: r.ID, OccurredAt: r.OccurredAt, Seq: r.Seq,
			ActorID: database.Deref(r.ActorID), ActorKind: r.ActorKind,
			TargetID: database.Deref(r.TargetID), SessionID: database.Deref(r.SessionID),
			Action: r.Action, Result: r.Result, IP: database.AuditIP(r.Ip),
			Detail: r.Detail, EntryHash: r.EntryHash,
		})
	}
	return out, nil
}

func (s *Repository) AuditChainRange(ctx context.Context, tenantID string, afterSeq int64, from, to *time.Time, batch int) ([]auditdomain.AuditRecord, error) {
	if batch <= 0 || batch > 5000 {
		batch = 1000
	}
	rows, err := s.q(ctx).AuditChainRange(ctx, gen.AuditChainRangeParams{
		TenantID: tenantID, AfterSeq: afterSeq,
		FromTs: from, ToTs: to, BatchSize: int32(batch),
	})
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]auditdomain.AuditRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, auditdomain.AuditRecord{
			OccurredAt: r.OccurredAt, Seq: r.Seq,
			ActorID: database.Deref(r.ActorID), ActorKind: r.ActorKind,
			TargetID: database.Deref(r.TargetID), SessionID: database.Deref(r.SessionID),
			Action: r.Action, Result: r.Result, IP: database.AuditIP(r.Ip),
			Detail: r.Detail, PrevHash: r.PrevHash, EntryHash: r.EntryHash,
		})
	}
	return out, nil
}
