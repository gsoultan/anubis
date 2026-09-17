package auditpg

import (
	"context"
	"time"

	"github.com/gsoultan/anubis/internal/audit/adapter/postgres/rgen/auditlog"
	auditrquery "github.com/gsoultan/anubis/internal/audit/adapter/postgres/rquery"
	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// QueryAudit is the console's log search.
//
// The optional filters are added only when they are set. Under sqlc each was
// `($n::uuid IS NULL OR actor_id = $n)`, which is one statement for every
// combination and asks the planner to handle a predicate that is usually
// inert; here an absent filter contributes no predicate at all, and storm
// compiles one statement per SHAPE, so the common searches stay prepared.
func (s *Repository) QueryAudit(ctx context.Context, tenantID string, q auditdomain.AuditQuery) ([]auditdomain.AuditRecord, error) {
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	b := auditlog.New().Where(auditlog.TenantID.Eq(tid))
	if q.ActorID != "" {
		actor, err := database.ParseUUID(q.ActorID)
		if err != nil {
			return nil, database.MapErr(err)
		}
		b = b.Where(auditlog.ActorID.Eq(actor))
	}
	b = b.WhereIf(q.Action != "", auditlog.Action.Eq(q.Action))
	if q.From != nil {
		b = b.Where(auditlog.OccurredAt.Gte(*q.From))
	}
	if q.To != nil {
		b = b.Where(auditlog.OccurredAt.Lt(*q.To))
	}
	if q.BeforeSeq != nil {
		b = b.Where(auditlog.Seq.Lt(*q.BeforeSeq))
	}
	rows, err := b.Order(auditlog.Seq.Desc()).Limit(int64(limit)).All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return records(rows, false), nil
}

// AuditChainRange walks seq order in batches for chain verification.
//
// Ascending and strictly after a cursor, because the verifier must see every
// link in order: a page that skipped one would verify the hashes it did read
// and report the chain intact.
func (s *Repository) AuditChainRange(ctx context.Context, tenantID string, afterSeq int64, from, to *time.Time, batch int) ([]auditdomain.AuditRecord, error) {
	if batch <= 0 || batch > 5000 {
		batch = 1000
	}
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	b := auditlog.New().
		Where(auditlog.TenantID.Eq(tid), auditlog.Seq.Gt(afterSeq))
	if from != nil {
		b = b.Where(auditlog.OccurredAt.Gte(*from))
	}
	if to != nil {
		b = b.Where(auditlog.OccurredAt.Lt(*to))
	}
	rows, err := b.Order(auditlog.Seq.Asc()).Limit(int64(batch)).All(ctx, s.ex(ctx), nil)
	if err != nil {
		return nil, database.MapErr(err)
	}
	return records(rows, true), nil
}

// records maps storm rows onto the domain type. withHashes carries prev_hash,
// which only the chain verifier needs — the console never displays it.
func records(rows []auditlog.Row, withHashes bool) []auditdomain.AuditRecord {
	out := make([]auditdomain.AuditRecord, 0, len(rows))
	for _, r := range rows {
		rec := auditdomain.AuditRecord{
			ID:         database.UUIDStr(r.ID),
			OccurredAt: r.OccurredAt,
			Seq:        r.Seq,
			ActorID:    optUUID(r.ActorID),
			ActorKind:  r.ActorKind,
			TargetID:   optUUID(r.TargetID),
			SessionID:  optUUID(r.SessionID),
			Action:     r.Action,
			Result:     r.Result,
			IP:         ipString(r),
			Detail:     []byte(r.Detail),
			EntryHash:  r.EntryHash,
		}
		if withHashes {
			rec.PrevHash = r.PrevHash
		}
		out = append(out, rec)
	}
	return out
}

// ipString renders inet the way the old query's host(ip)::text did: the
// address without the mask, and "" when absent.
func ipString(r auditlog.Row) string {
	if !r.IP.Valid {
		return ""
	}
	return r.IP.V.Addr().String()
}

func optUUID(v interface{ Get() ([16]byte, bool) }) string {
	if u, ok := v.Get(); ok {
		return database.UUIDStr(u)
	}
	return ""
}

// CountDecisions24h backs the console's overview. Sampled under pressure,
// so floors rather than exact counts — the screen says "24h", not "exact".
func (s *Repository) CountDecisions24h(ctx context.Context, tenantID string) (allows, denies int64, err error) {
	row, _, err := auditrquery.CountDecisions24h.One(ctx, s.ex(ctx), tenantID)
	if err != nil {
		return 0, 0, database.MapErr(err)
	}
	return row.Allows, row.Denies, nil
}

// ReuseSignal reports stolen-token events over the last week.
func (s *Repository) ReuseSignal(ctx context.Context, tenantID string) (int64, time.Time, error) {
	row, _, err := auditrquery.ReuseSignal.One(ctx, s.ex(ctx), tenantID)
	if err != nil {
		return 0, time.Time{}, database.MapErr(err)
	}
	return row.N, row.Latest, nil
}
