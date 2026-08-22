package auditpg

import (
	"context"
	"time"

	gen "github.com/gsoultan/anubis/internal/audit/adapter/postgres/gen"
	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func (s *Repository) LockAuditChain(ctx context.Context, tenantID string) error {
	return database.MapErr(s.q(ctx).AdvisoryLockAuditChain(ctx, tenantID))
}

func (s *Repository) LastAuditEntry(ctx context.Context, tenantID string) (int64, []byte, error) {
	row, err := s.q(ctx).LastAuditEntry(ctx, tenantID)
	if err != nil {
		if apperr.AsError(database.MapErr(err)).Code == apperr.ErrNotFound.Code {
			return 0, nil, nil // empty chain starts at seq 0
		}
		return 0, nil, database.MapErr(err)
	}
	return row.Seq, row.EntryHash, nil
}

func (s *Repository) InsertAudit(ctx context.Context, ev auditdomain.AuditEvent, seq int64, prevHash, entryHash []byte) (time.Time, error) {
	row, err := s.q(ctx).InsertAudit(ctx, gen.InsertAuditParams{
		TenantID:  ev.TenantID,
		ActorID:   database.OptStr(ev.ActorID),
		ActorKind: ev.ActorKind,
		TargetID:  database.OptStr(ev.TargetID),
		SessionID: database.OptStr(ev.SessionID),
		Seq:       seq,
		Action:    ev.Action,
		Result:    ev.Result,
		Ip:        ev.IP,
		Detail:    database.OrEmptyJSON(ev.Detail),
		PrevHash:  prevHash,
		EntryHash: entryHash,
	})
	if err != nil {
		return time.Time{}, database.MapErr(err)
	}
	return row.OccurredAt, nil
}
