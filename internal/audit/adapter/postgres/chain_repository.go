package auditpg

import (
	"context"

	"github.com/gsoultan/anubis/internal/audit/adapter/postgres/rgen/auditlog"
	auditrquery "github.com/gsoultan/anubis/internal/audit/adapter/postgres/rquery"
	"github.com/gsoultan/anubis/internal/platform/database"
)

// Anchor is a signed statement that a tenant's chain reached Seq with
// EntryHash.
type Anchor struct {
	Seq       int64
	EntryHash []byte
	Kid       string
	Signature []byte
}

// InsertAnchor records one. Re-anchoring a head that is already anchored is
// a no-op rather than an overwrite.
func (s *Repository) InsertAnchor(ctx context.Context, tenantID string, a Anchor) error {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return database.MapErr(err)
	}
	_, err = auditrquery.InsertAuditAnchor.Exec(ctx, s.ex(ctx),
		tid, a.Seq, a.EntryHash, a.Kid, a.Signature)
	return database.MapErr(err)
}

// AnchorsFrom lists a tenant's anchors at or after seq, oldest first.
func (s *Repository) AnchorsFrom(ctx context.Context, tenantID string, seq int64) ([]Anchor, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	rows, err := auditrquery.AuditAnchorsFrom.Query(ctx, s.ex(ctx), tid, seq)
	if err != nil {
		return nil, database.MapErr(err)
	}
	out := make([]Anchor, len(rows))
	for i, r := range rows {
		out[i] = Anchor{Seq: r.Seq, EntryHash: r.EntryHash, Kid: r.Kid, Signature: r.Signature}
	}
	return out, nil
}

// AuditEntryHashAt returns the entry_hash currently stored at one sequence.
//
// Deliberately reads the row as it is NOW rather than recomputing it: an
// anchor's job is to catch a chain that was rewritten into self-consistency,
// and recomputing would agree with the rewrite.
func (s *Repository) AuditEntryHashAt(ctx context.Context, tenantID string, seq int64) ([]byte, error) {
	tid, err := database.ParseUUID(tenantID)
	if err != nil {
		return nil, database.MapErr(err)
	}
	row, ok, err := auditlog.New().
		Where(auditlog.TenantID.Eq(tid), auditlog.Seq.Eq(seq)).
		One(ctx, s.ex(ctx))
	if err != nil {
		return nil, database.MapErr(err)
	}
	if !ok {
		// The anchored entry is GONE. That is tampering, not an empty
		// result, so it must not read as "hash matched nothing".
		return nil, nil
	}
	return row.EntryHash, nil
}
