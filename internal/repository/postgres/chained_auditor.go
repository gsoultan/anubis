package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"log/slog"
	"time"

	"github.com/gsoultan/anubis/internal/repository"
)

// ChainedAuditor implements repository.Auditor with the per-tenant hash
// chain. Writes are asynchronous through a bounded queue: security events
// block when the queue is full (they must not be lost); sampled authorize
// events drop with a counter (they are a statistical record).
type ChainedAuditor struct {
	store   *Store
	logger  *slog.Logger
	queue   chan repository.AuditEvent
	dropped uint64
	done    chan struct{}
}

func NewChainedAuditor(store *Store, logger *slog.Logger) *ChainedAuditor {
	a := &ChainedAuditor{
		store:  store,
		logger: logger,
		queue:  make(chan repository.AuditEvent, 4096),
		done:   make(chan struct{}),
	}
	go a.run()
	return a
}

func (a *ChainedAuditor) Emit(ctx context.Context, ev repository.AuditEvent) {
	if ev.TenantID == "" {
		return
	}
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
			a.logger.Error("audit append failed", "action", ev.Action, "error", err)
		}
	}
}

// append serialises per tenant with an advisory transaction lock, reads the
// chain head, and writes seq+1 with entry_hash = H(prev || canonical fields).
func (a *ChainedAuditor) append(ctx context.Context, ev repository.AuditEvent) error {
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

// chainHash is the canonical hash every verifier must reproduce:
// sha256(prev_hash || be64(seq) || tenant || action || result || actor ||
// target || session || detail).
func chainHash(prev []byte, seq int64, ev repository.AuditEvent) []byte {
	h := sha256.New()
	h.Write(prev)
	var seqB [8]byte
	binary.BigEndian.PutUint64(seqB[:], uint64(seq))
	h.Write(seqB[:])
	for _, s := range []string{ev.TenantID, ev.Action, ev.Result, ev.ActorID, ev.TargetID, ev.SessionID} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	h.Write(ev.Detail)
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
			want := chainHash(prev, r.Seq, repository.AuditEvent{
				TenantID: tenantID, Action: r.Action, Result: r.Result,
				ActorID: r.ActorID, TargetID: r.TargetID, SessionID: r.SessionID,
				Detail: r.Detail,
			})
			if !equalBytes(want, r.EntryHash) {
				return checked, r.Seq, nil
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
