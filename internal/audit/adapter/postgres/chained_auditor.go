package auditpg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"log/slog"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	"github.com/gsoultan/anubis/internal/platform/metrics"
)

// ChainedAuditor implements auditport.Auditor with the per-tenant hash
// chain. Writes are asynchronous through a bounded queue: security events
// block when the queue is full (they must not be lost); sampled authorize
// events drop with a counter (they are a statistical record).
type ChainedAuditor struct {
	store   *Repository
	logger  *slog.Logger
	queue   chan auditdomain.AuditEvent
	dropped uint64
	done    chan struct{}
}

func NewChainedAuditor(store *Repository, logger *slog.Logger) *ChainedAuditor {
	a := &ChainedAuditor{
		store:  store,
		logger: logger,
		queue:  make(chan auditdomain.AuditEvent, 4096),
		done:   make(chan struct{}),
	}
	go a.run()
	return a
}

func (a *ChainedAuditor) Emit(ctx context.Context, ev auditdomain.AuditEvent) {
	if ev.TenantID == "" {
		return
	}
	// Counted at emission, before any queueing or sampling: the metric asks
	// "did the system observe this", and token.reuse_detected is the pager.
	metrics.IncAudit(ev.Action)
	if ev.Action == "authorize" {
		select {
		case a.queue <- ev:
		default:
			a.dropped++ // sampled events may drop under pressure
		}
		return
	}
	select {
	case a.queue <- ev:
	case <-ctx.Done():
		// Even on caller cancellation the event matters; last try without ctx.
		a.queue <- ev
	}
}

// Close drains the queue; call on shutdown.
func (a *ChainedAuditor) Close() {
	close(a.queue)
	<-a.done
}

func (a *ChainedAuditor) run() {
	defer close(a.done)
	for ev := range a.queue {
		if err := a.append(context.Background(), ev); err != nil {
			// A log line is not enough for this one. The audit log is the
			// artefact a regulator reads; an entry that never lands makes it
			// incomplete, and nobody greps yesterday's logs to find out.
			metrics.IncAuditDropped(ev.Action)
			a.logger.Error("audit append failed", "action", ev.Action, "error", err)
		}
	}
}

// append serialises per tenant with an advisory transaction lock, reads the
// chain head, and writes seq+1 with entry_hash = H(prev || canonical fields).
func (a *ChainedAuditor) append(ctx context.Context, ev auditdomain.AuditEvent) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return a.store.WithinTx(ctx, func(ctx context.Context) error {
		if err := a.store.LockAuditChain(ctx, ev.TenantID); err != nil {
			return err
		}
		seq, prevHash, err := a.store.LastAuditEntry(ctx, ev.TenantID)
		if err != nil {
			return err
		}
		next := seq + 1
		entryHash := chainHash(prevHash, next, ev)
		_, err = a.store.InsertAudit(ctx, ev, next, prevHash, entryHash)
		return err
	})
}

// canonicalJSON renders a detail the way BOTH sides can agree on.
//
// detail is a jsonb column, and Postgres does not store the bytes it is
// given: it parses and re-renders them, inserting a space after every colon
// and reordering keys. So hashing what the writer happened to send produces
// a hash the reader can never reproduce — every entry with a detail reads as
// tampered. Parsing and re-marshalling on both sides removes the difference,
// because whatever jsonb did to the formatting is undone before hashing.
//
// An absent detail and an empty object collapse to the same thing, because
// the column cannot tell them apart: a writer that passes nil and one that
// passes {} both leave {} in the row. The hash must make the same
// identification or every detail-less entry fails to verify — which is what
// the audit log's own history does, and what proved this rule rather than
// assuming it.
//
// UseNumber keeps numeric literals as written rather than routing them
// through float64, where a large integer would come back a different number
// than it went in. A detail that is not valid JSON is hashed as-is: it
// should not exist, and silently hashing something else would hide it.
func canonicalJSON(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return b
	}
	out, err := json.Marshal(v)
	if err != nil {
		return b
	}
	if bytes.Equal(out, []byte("{}")) {
		return nil
	}
	return out
}

// chainHash is the canonical hash every verifier must reproduce:
// sha256(prev_hash || be64(seq) || tenant || action || result || actor ||
// target || session || canonical(detail)).
func chainHash(prev []byte, seq int64, ev auditdomain.AuditEvent) []byte {
	return chainHashDetail(prev, seq, ev, canonicalJSON(ev.Detail))
}

// chainHashDetail is chainHash with the detail bytes supplied, so
// verification can test a second reading of a detail-less entry — see
// VerifyChain.
func chainHashDetail(prev []byte, seq int64, ev auditdomain.AuditEvent, detail []byte) []byte {
	h := sha256.New()
	h.Write(prev)
	var seqB [8]byte
	binary.BigEndian.PutUint64(seqB[:], uint64(seq))
	h.Write(seqB[:])
	for _, s := range []string{ev.TenantID, ev.Action, ev.Result, ev.ActorID, ev.TargetID, ev.SessionID} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	h.Write(detail)
	return h.Sum(nil)
}

// VerifyChain walks [afterSeq..] recomputing hashes; returns the first seq
// where the chain breaks, or 0 when intact.
func (a *ChainedAuditor) VerifyChain(ctx context.Context, tenantID string, from, to *time.Time) (checked int64, brokenAt int64, err error) {
	var prev []byte
	after := int64(0)
	first := true
	for {
		batch, err := a.store.AuditChainRange(ctx, tenantID, after, from, to, 1000)
		if err != nil {
			return checked, 0, err
		}
		if len(batch) == 0 {
			return checked, 0, nil
		}
		for _, r := range batch {
			if first {
				// The first row in range anchors the walk; trust its prev.
				prev = r.PrevHash
				first = false
			}
			want := chainHash(prev, r.Seq, auditdomain.AuditEvent{
				TenantID: tenantID, Action: r.Action, Result: r.Result,
				ActorID: r.ActorID, TargetID: r.TargetID, SessionID: r.SessionID,
				Detail: r.Detail,
			})
			if !equalBytes(want, r.EntryHash) {
				// A detail-less entry has two possible readings and the row
				// cannot say which it was: early call sites passed an empty
				// object where later ones passed nothing, and jsonb stores {}
				// either way. Accepting both is the price of verifying history
				// written before the two were reconciled; it costs an attacker
				// nothing less than a second SHA-256 preimage, and only for
				// entries that carry no detail to forge.
				if len(canonicalJSON(r.Detail)) != 0 {
					return checked, r.Seq, nil
				}
				alt := chainHashDetail(prev, r.Seq, auditdomain.AuditEvent{
					TenantID: tenantID, Action: r.Action, Result: r.Result,
					ActorID: r.ActorID, TargetID: r.TargetID, SessionID: r.SessionID,
				}, []byte("{}"))
				if !equalBytes(alt, r.EntryHash) {
					return checked, r.Seq, nil
				}
			}
			prev = r.EntryHash
			checked++
			after = r.Seq
		}
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
