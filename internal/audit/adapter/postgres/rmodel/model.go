// Package auditrmodel declares the audit tables storm generates code for.
//
// It is a PROJECTION of the schema, not its source of truth: anubis's schema
// of record stays migrations/ (forward-only, checksummed), and this model is
// validated against the live schema the same way the raw queries are — by
// preparing generated statements against it, and by the integration suite.
// Columns and defaults here must match `\d` on the table exactly.
package auditrmodel

import (
	"net/netip"
	"time"

	"github.com/gsoultan/storm"
)

// AuditLog is public.audit_log, the append-only hash chain.
//
// It is RANGE partitioned on occurred_at, one partition per month, and the
// partitions are created by ensure_month_partitions() rather than by any
// model — storm knows the parent and deliberately leaves the partitions
// alone, so a diff never proposes dropping last month's entries.
//
// The primary key carries occurred_at because PostgreSQL requires the
// partition key in every unique key on a partitioned table. It is not a
// statement that (id, occurred_at) is the identity; id alone is.
type AuditLog struct {
	OccurredAt time.Time
	ID         [16]byte
	TenantID   [16]byte
	ActorID    *[16]byte
	TargetID   *[16]byte
	SessionID  *[16]byte
	Seq        int64
	Action     string
	Result     string
	ActorKind  string
	IP         *netip.Prefix
	Detail     storm.JSON

	// PrevHash is null for the first entry of a tenant's chain and set for
	// every one after it. EntryHash is never null — it is the tamper evidence,
	// and a row without one is a link that cannot be checked. []byte is
	// nullable to storm, so that has to be said rather than implied.
	PrevHash  []byte
	EntryHash []byte
}

func (m *AuditLog) Schema(t *storm.Table) {
	t.Name("audit_log")
	t.PrimaryKey(&m.ID, &m.OccurredAt)
	t.PartitionBy(storm.RangePartition, &m.OccurredAt)

	t.Col(&m.OccurredAt).Default("now()")
	t.Col(&m.ID).Default("uuidv7()")
	t.Col(&m.ActorKind).Default("'identity'::text")
	t.Col(&m.Detail).Default("'{}'::jsonb")
	t.Col(&m.EntryHash).NotNull()

	t.CheckNamed("audit_log_result_check",
		"result = ANY (ARRAY['allow'::text, 'deny'::text, 'error'::text])")

	t.Index(&m.TenantID, storm.Desc(&m.Seq)).Named("audit_log_tenant_seq")
	t.Index(&m.TenantID, &m.Action, storm.Desc(&m.OccurredAt)).Named("audit_log_tenant_action")
	t.Index(&m.ActorID, storm.Desc(&m.OccurredAt)).
		Where("actor_id IS NOT NULL").Named("audit_log_actor")
	// BRIN, not btree: the table is append-only in occurred_at order, so a
	// summary per 32-page range answers a time window at a fraction of the
	// size. A btree here would be larger than the rows it indexes.
	t.Index(&m.OccurredAt).Using(storm.BRIN).
		With("pages_per_range", "32").Named("audit_log_time_brin")
}

func All() []any { return []any{&AuditLog{}} }
