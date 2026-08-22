package postgres

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/adapter/postgres/gen"
	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

func (s *Store) LockAuditChain(ctx context.Context, tenantID string) error {
	return mapErr(s.q(ctx).AdvisoryLockAuditChain(ctx, tenantID))
}

func (s *Store) LastAuditEntry(ctx context.Context, tenantID string) (int64, []byte, error) {
	row, err := s.q(ctx).LastAuditEntry(ctx, tenantID)
	if err != nil {
		if domain.AsError(mapErr(err)).Code == domain.ErrNotFound.Code {
			return 0, nil, nil // empty chain starts at seq 0
		}
		return 0, nil, mapErr(err)
	}
	return row.Seq, row.EntryHash, nil
}

func (s *Store) InsertAudit(ctx context.Context, ev repository.AuditEvent, seq int64, prevHash, entryHash []byte) (time.Time, error) {
	row, err := s.q(ctx).InsertAudit(ctx, gen.InsertAuditParams{
		TenantID:  ev.TenantID,
		ActorID:   optStr(ev.ActorID),
		ActorKind: ev.ActorKind,
		TargetID:  optStr(ev.TargetID),
		SessionID: optStr(ev.SessionID),
		Seq:       seq,
		Action:    ev.Action,
		Result:    ev.Result,
		Ip:        ev.IP,
		Detail:    orEmptyJSON(ev.Detail),
		PrevHash:  prevHash,
		EntryHash: entryHash,
	})
	if err != nil {
		return time.Time{}, mapErr(err)
	}
	return row.OccurredAt, nil
}
