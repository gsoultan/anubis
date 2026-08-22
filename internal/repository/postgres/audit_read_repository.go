package postgres

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) QueryAudit(ctx context.Context, tenantID string, q repository.AuditQuery) ([]repository.AuditRecord, error) {
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var beforeSeq *int64 = q.BeforeSeq
	rows, err := s.q(ctx).QueryAudit(ctx, gen.QueryAuditParams{
		TenantID: tenantID,
		ActorID:  optStr(q.ActorID),
		Action:   optStr(q.Action),
		FromTs:   q.From,
		ToTs:     q.To,
		BeforeSeq: beforeSeq,
		PageSize:  int32(limit),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.AuditRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.AuditRecord{
			ID: r.ID, OccurredAt: r.OccurredAt, Seq: r.Seq,
			ActorID: deref(r.ActorID), ActorKind: r.ActorKind,
			TargetID: deref(r.TargetID), SessionID: deref(r.SessionID),
			Action: r.Action, Result: r.Result, IP: auditIP(r.Ip),
			Detail: r.Detail, EntryHash: r.EntryHash,
		})
	}
	return out, nil
}

func (s *Store) AuditChainRange(ctx context.Context, tenantID string, afterSeq int64, from, to *time.Time, batch int) ([]repository.AuditRecord, error) {
	if batch <= 0 || batch > 5000 {
		batch = 1000
	}
	rows, err := s.q(ctx).AuditChainRange(ctx, gen.AuditChainRangeParams{
		TenantID: tenantID, AfterSeq: afterSeq,
		FromTs: from, ToTs: to, BatchSize: int32(batch),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]repository.AuditRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.AuditRecord{
			OccurredAt: r.OccurredAt, Seq: r.Seq,
			ActorID: deref(r.ActorID), ActorKind: r.ActorKind,
			TargetID: deref(r.TargetID), SessionID: deref(r.SessionID),
			Action: r.Action, Result: r.Result, IP: auditIP(r.Ip),
			Detail: r.Detail, PrevHash: r.PrevHash, EntryHash: r.EntryHash,
		})
	}
	return out, nil
}
