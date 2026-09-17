package auditpg

import (
	"context"
	"net/netip"
	"time"

	"github.com/gsoultan/anubis/internal/audit/adapter/postgres/rgen/auditlog"
	auditrquery "github.com/gsoultan/anubis/internal/audit/adapter/postgres/rquery"
	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/storm/runtime"
)

// LockAuditChain serialises this tenant's chain appends for the rest of the
// transaction, so two writers cannot both read seq N and both write N+1.
func (s *Repository) LockAuditChain(ctx context.Context, tenantID string) error {
	if _, _, err := auditrquery.AdvisoryLockAuditChain.One(ctx, s.ex(ctx), tenantID); err != nil {
		return database.MapErr(err)
	}
	return nil
}

// LastAuditEntry returns the tip of a tenant's chain, or (0, nil) when the
// chain is empty — a new tenant's first entry starts at seq 0 rather than
// erroring, and "no rows" is that case, not a failure.
func (s *Repository) LastAuditEntry(ctx context.Context, tenantID string) (int64, []byte, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return 0, nil, database.MapErr(err)
	}
	row, ok, err := auditlog.New().
		Where(auditlog.TenantID.Eq(tid)).
		Order(auditlog.Seq.Desc()).
		Limit(1).
		One(ctx, s.ex(ctx))
	if err != nil {
		return 0, nil, database.MapErr(err)
	}
	if !ok {
		return 0, nil, nil // empty chain starts at seq 0
	}
	return row.Seq, row.EntryHash, nil
}

// InsertAudit appends one entry. The caller holds the chain lock and has
// already computed seq and the hashes; this only writes them.
func (s *Repository) InsertAudit(ctx context.Context, ev auditdomain.AuditEvent, seq int64, prevHash, entryHash []byte) (time.Time, error) {
	tid, err := database.ParseUUID(ev.TenantID)
	if err != nil {
		return time.Time{}, database.MapErr(err)
	}
	n := auditlog.Create()
	n.SetTenantID(tid)
	n.SetSeq(seq)
	n.SetAction(ev.Action)
	n.SetResult(ev.Result)
	n.SetActorKind(ev.ActorKind)
	n.SetEntryHash(entryHash)

	// Unset columns take their database defaults, so occurred_at and id are
	// left alone deliberately: now() and uuidv7() belong to the schema, and
	// an appender that supplied its own clock would let a caller's skew
	// reorder the chain it is meant to be proving.
	if err := setOptUUID(n.SetActorID, n.SetActorIDNull, ev.ActorID); err != nil {
		return time.Time{}, database.MapErr(err)
	}
	if err := setOptUUID(n.SetTargetID, n.SetTargetIDNull, ev.TargetID); err != nil {
		return time.Time{}, database.MapErr(err)
	}
	if err := setOptUUID(n.SetSessionID, n.SetSessionIDNull, ev.SessionID); err != nil {
		return time.Time{}, database.MapErr(err)
	}
	// An empty IP is absent, not 0.0.0.0/0: the column is nullable precisely
	// so "we do not know" and "from everywhere" stay different answers. Under
	// sqlc this was nullif($n,'')::inet in the SQL; the decision is the same
	// one, made where the value is.
	if p, ok, err := parseIP(ev.IP); err != nil {
		return time.Time{}, database.MapErr(err)
	} else if ok {
		n.SetIP(p)
	} else {
		n.SetIPNull()
	}
	n.SetDetail(runtime.JSON(database.OrEmptyJSON(ev.Detail)))
	if prevHash != nil {
		n.SetPrevHash(prevHash)
	}

	row, err := n.Insert(ctx, s.ex(ctx))
	if err != nil {
		return time.Time{}, database.MapErr(err)
	}
	return row.OccurredAt, nil
}

// setOptUUID writes an optional id, treating "" as absent.
func setOptUUID(set func([16]byte), setNull func(), v string) error {
	if v == "" {
		setNull()
		return nil
	}
	u, err := database.ParseUUID(v)
	if err != nil {
		return err
	}
	set(u)
	return nil
}

// parseIP accepts either a bare address or a CIDR, because the column is inet
// and both are things an audit event legitimately carries.
func parseIP(s string) (netip.Prefix, bool, error) {
	if s == "" {
		return netip.Prefix{}, false, nil
	}
	if p, err := netip.ParsePrefix(s); err == nil {
		return p, true, nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, false, err
	}
	return netip.PrefixFrom(a, a.BitLen()), true, nil
}
