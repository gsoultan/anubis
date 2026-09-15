package scopeapp

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	"github.com/gsoultan/anubis/internal/authz/guard"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	scopeport "github.com/gsoultan/anubis/internal/scope/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/gsoultan/anubis/internal/shared/txm"
	"github.com/gsoultan/anubis/internal/shared/validate"
)

// scopeAdminInteractor implements ScopeAdmin.
type scopeAdminInteractor struct {
	logger  *slog.Logger
	guard   *guard.Guard
	axes    scopeport.ScopeAxisRepository
	nodes   scopeport.ScopeNodeRepository
	sync    scopeport.ScopeSyncRepository
	authz   authzport.AuthzRepository
	fetcher scopeport.ScopeFeedFetcher
	tx      txm.TxManager
	audit   auditport.Auditor
}

func NewScopeAdminInteractor(
	// authz is NOT for the guard (administration is operator-only): strict
	// dry runs replay real authorize decisions to predict what flipping an
	// axis to strict would deny.
	authz authzport.AuthzRepository,
	// ops lets a PLATFORM OPERATOR do this inside a tenant they are assigned
	// to (ADR-0011): their authority is an assignment, not a grant.
	ops guard.OperatorAuthority,
	clockNow func() time.Time,
	axes scopeport.ScopeAxisRepository,
	nodes scopeport.ScopeNodeRepository,
	sync scopeport.ScopeSyncRepository,
	fetcher scopeport.ScopeFeedFetcher,
	tx txm.TxManager,
	audit auditport.Auditor,
	logger *slog.Logger,
) ScopeAdmin {
	return &scopeAdminInteractor{
		guard: guard.New().WithOperators(ops, clockNow), axes: axes, nodes: nodes, sync: sync,
		authz: authz, fetcher: fetcher, tx: tx, audit: audit, logger: logger,
	}
}

// emit records an administrative change. target must be a UUID or empty:
// audit_log.target_id is a uuid column, and an axis or node type is
// identified by a code, not an id. Those belong in detail — passing a code
// here silently threw the entry away for as long as this code existed.
func (u *scopeAdminInteractor) emit(ctx context.Context, p *authctx.Principal, action, target string, detail map[string]string) {
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, TargetID: target, Action: action, Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: jsonx.Must(detail),
	})
}

// emitSync records a sync event for either kind of caller. p is nil for a
// scheduled run, which then audits as actor_kind='system' with no actor id.
// The shape that must never occur is a timer's run wearing an operator's
// identity because the code only knew how to write one kind of entry.
func (u *scopeAdminInteractor) emitSync(ctx context.Context, tenantID string,
	p *authctx.Principal, action, target string, detail map[string]string,
) {
	ev := auditdomain.AuditEvent{
		TenantID: tenantID, ActorKind: "system", TargetID: target,
		Action: action, Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: jsonx.Must(detail),
	}
	if p != nil {
		ev.ActorID, ev.ActorKind, ev.SessionID = p.IdentityID, "identity", p.SessionID
	}
	u.audit.Emit(ctx, ev)
}

func (u *scopeAdminInteractor) ListScopeAxes(ctx context.Context) ([]scopedomain.ScopeAxisRecord, error) {
	if _, err := u.guard.Require(ctx, "anubis:identity:read"); err != nil {
		return nil, err
	}
	return u.axes.ListScopeAxes(ctx)
}

func (u *scopeAdminInteractor) CreateScopeAxis(ctx context.Context, a scopedomain.ScopeAxisRecord) (*scopedomain.ScopeAxisRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:scope:admin")
	if err != nil {
		return nil, err
	}
	if !validate.ValidCode(a.Code) {
		return nil, apperr.ErrInvalidArgument.With("code", a.Code)
	}
	if err := u.axes.CreateScopeAxis(ctx, a); err != nil {
		return nil, err
	}
	u.emit(ctx, p, "scope.axis_create", "", map[string]string{"axis": a.Code, "effect": a.DefaultEffect})
	return u.axes.ScopeAxis(ctx, a.Code)
}

func (u *scopeAdminInteractor) UpdateScopeAxis(ctx context.Context, a scopedomain.ScopeAxisRecord) (*scopedomain.ScopeAxisRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:scope:admin")
	if err != nil {
		return nil, err
	}
	if err := u.axes.UpdateScopeAxis(ctx, a); err != nil {
		return nil, err
	}
	u.emit(ctx, p, "scope.axis_update", "", map[string]string{"axis": a.Code, "effect": a.DefaultEffect, "status": a.Status})
	return u.axes.ScopeAxis(ctx, a.Code)
}

// StrictDryRun: replay recent allow decisions against a hypothetically-strict
// axis. On the seed data the flip took allows 800 -> 0 — better learned from
// this report than an outage (docs/api.md).
func (u *scopeAdminInteractor) StrictDryRun(ctx context.Context, axis string, sampleSize int) (int, int, string, error) {
	p, err := u.guard.Require(ctx, "anubis:scope:admin")
	if err != nil {
		return 0, 0, "", err
	}
	if sampleSize <= 0 || sampleSize > 10000 {
		sampleSize = 2000
	}
	samples, err := u.authz.SampleAuthorizeDecisions(ctx, p.TenantID, sampleSize)
	if err != nil {
		return 0, 0, "", err
	}
	type sample struct {
		Subject    string            `json:"subject"`
		Permission string            `json:"permission"`
		Targets    map[string]string `json:"targets"`
	}
	wouldDeny := 0
	sampled := 0
	examples := make([]sample, 0, 20)
	for _, raw := range samples {
		var s sample
		if json.Unmarshal(raw, &s) != nil || s.Subject == "" || s.Permission == "" {
			continue
		}
		sampled++
		targets, _ := json.Marshal(s.Targets)
		allow, serr := u.authz.AuthorizeStrictSim(ctx, s.Subject, p.TenantID, s.Permission, targets, axis)
		if serr != nil {
			return 0, 0, "", serr
		}
		if !allow {
			wouldDeny++
			if len(examples) < 20 {
				examples = append(examples, s)
			}
		}
	}
	raw, _ := json.Marshal(examples)
	return sampled, wouldDeny, string(raw), nil
}

func (u *scopeAdminInteractor) ListScopeNodeTypes(ctx context.Context, axis string) ([]scopedomain.ScopeNodeTypeRecord, error) {
	if _, err := u.guard.Require(ctx, "anubis:identity:read"); err != nil {
		return nil, err
	}
	return u.nodes.ListScopeNodeTypes(ctx, axis)
}

func (u *scopeAdminInteractor) CreateScopeNodeType(ctx context.Context, t scopedomain.ScopeNodeTypeRecord) error {
	p, err := u.guard.Require(ctx, "anubis:scope:admin")
	if err != nil {
		return err
	}
	if err := u.nodes.CreateScopeNodeType(ctx, t); err != nil {
		return err
	}
	u.emit(ctx, p, "scope.node_type_create", "", map[string]string{"axis": t.Axis, "node_type": t.Code})
	return nil
}

func (u *scopeAdminInteractor) ListScopeNodes(ctx context.Context, f scopedomain.ScopeNodeFilter) ([]scopedomain.ScopeNodeRecord, string, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, "", err
	}
	f = f.Normalise()
	page, err := u.nodes.ListScopeNodes(ctx, p.TenantID, f)
	if err != nil {
		return nil, "", err
	}
	// A full page MAY have more behind it; the client gets one empty final
	// page rather than us spending a second query to prove otherwise.
	var next string
	if int32(len(page)) == f.Limit && len(page) > 0 {
		next = page[len(page)-1].PageToken()
	}
	return page, next, nil
}

// eachScopeNode walks EVERY node matching f, one keyset page at a time.
//
// The archive pass below depends on seeing all of them: it archives
// sync-owned nodes the feed no longer contains, so a truncated walk silently
// keeps deleted nodes — and the grants hanging off them — alive. That is what
// the old hard LIMIT 2000 did on any axis larger than that.
func (u *scopeAdminInteractor) eachScopeNode(ctx context.Context, tenantID string,
	f scopedomain.ScopeNodeFilter, fn func(scopedomain.ScopeNodeRecord) error) error {
	f = f.Normalise()
	for {
		page, err := u.nodes.ListScopeNodes(ctx, tenantID, f)
		if err != nil {
			return err
		}
		for _, n := range page {
			if err := fn(n); err != nil {
				return err
			}
		}
		if int32(len(page)) < f.Limit {
			return nil // short page = last page
		}
		f = f.After(page[len(page)-1])
	}
}

func (u *scopeAdminInteractor) CreateScopeNode(ctx context.Context, axis, nodeType, parentID, slug, name, externalRef string) (*scopedomain.ScopeNodeRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:scope:admin")
	if err != nil {
		return nil, err
	}
	if parentID == "" {
		root, rerr := u.nodes.EnsureAxisRoot(ctx, p.TenantID, axis)
		if rerr != nil {
			return nil, rerr
		}
		parentID = root
	}
	if slug == "" {
		slug = slugify(name)
	}
	id, err := u.nodes.AddScopeNode(ctx, p.TenantID, axis, nodeType, parentID, slug, name, externalRef)
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "scope.node_create", id, map[string]string{"axis": axis, "name": name})
	return u.nodes.ScopeNode(ctx, p.TenantID, id)
}

func (u *scopeAdminInteractor) EnsureAxisRoot(ctx context.Context, axis string) (string, error) {
	p, err := u.guard.Require(ctx, "anubis:scope:admin")
	if err != nil {
		return "", err
	}
	return u.nodes.EnsureAxisRoot(ctx, p.TenantID, axis)
}

func (u *scopeAdminInteractor) MoveScopeNode(ctx context.Context, nodeID, newParentID string) error {
	p, err := u.guard.Require(ctx, "anubis:scope:admin")
	if err != nil {
		return err
	}
	// Tenancy: both ends must live in the caller's tenant before the move.
	if _, err := u.nodes.ScopeNode(ctx, p.TenantID, nodeID); err != nil {
		return apperr.ErrNotFound
	}
	if _, err := u.nodes.ScopeNode(ctx, p.TenantID, newParentID); err != nil {
		return apperr.ErrNotFound
	}
	if err := u.nodes.MoveScopeNode(ctx, nodeID, newParentID); err != nil {
		return err
	}
	u.emit(ctx, p, "scope.node_move", nodeID, map[string]string{"new_parent": newParentID})
	return nil
}

func (u *scopeAdminInteractor) ArchiveScopeNode(ctx context.Context, nodeID string) error {
	p, err := u.guard.Require(ctx, "anubis:scope:admin")
	if err != nil {
		return err
	}
	if err := u.nodes.ArchiveScopeNode(ctx, p.TenantID, nodeID); err != nil {
		return err
	}
	u.emit(ctx, p, "scope.node_archive", nodeID, nil)
	return nil
}

// UpsertScopeNodes: bulk reconcile keyed on external_ref. Adds, renames and
// moves run through the schema functions so the 0014 level guard still bites;
// nodes missing from the feed archive only when they carry a ref (manual
// nodes are never sync's to remove).
func (u *scopeAdminInteractor) UpsertScopeNodes(ctx context.Context, axis, defaultNodeType string, rows []SyncRowInput, dry bool) (string, error) {
	p, err := u.guard.Require(ctx, "anubis:scope:admin")
	if err != nil {
		return "", err
	}
	if len(rows) > 50000 {
		return "", apperr.ErrInvalidArgument.With("rows", "max 50000 per call")
	}
	report := map[string]any{"dry": dry}
	added, renamed, moved, unchanged := 0, 0, 0, 0
	archived := 0
	errs := []map[string]string{}

	run := func(ctx context.Context) error {
		root, rerr := u.nodes.EnsureAxisRoot(ctx, p.TenantID, axis)
		if rerr != nil {
			return rerr
		}
		seen := make(map[string]bool, len(rows))
		for _, row := range rows {
			if row.Ref == "" || row.Name == "" {
				errs = append(errs, map[string]string{"ref": row.Ref, "error": "missing ref or name"})
				continue
			}
			seen[row.Ref] = true
			parent := root
			if row.ParentRef != "" {
				pn, perr := u.nodes.ScopeNodeByRef(ctx, p.TenantID, axis, row.ParentRef)
				if perr != nil {
					errs = append(errs, map[string]string{"ref": row.Ref, "error": "parent ref not found (parents must come first)"})
					continue
				}
				parent = pn.ID
			}
			nodeType := row.NodeType
			if nodeType == "" {
				nodeType = defaultNodeType
			}
			existing, gerr := u.nodes.ScopeNodeByRef(ctx, p.TenantID, axis, row.Ref)
			switch {
			case gerr != nil: // new node
				if _, aerr := u.nodes.AddScopeNode(ctx, p.TenantID, axis, nodeType,
					parent, slugify(row.Name)+"-"+suffix(row.Ref), row.Name, row.Ref); aerr != nil {
					errs = append(errs, map[string]string{"ref": row.Ref, "error": apperr.AsError(aerr).Message})
					continue
				}
				added++
			case existing.ParentID != parent:
				if merr := u.nodes.MoveScopeNode(ctx, existing.ID, parent); merr != nil {
					errs = append(errs, map[string]string{"ref": row.Ref, "error": apperr.AsError(merr).Message})
					continue
				}
				moved++
			case existing.Name != row.Name || existing.Status == "archived":
				if rerr := u.nodes.RenameScopeNode(ctx, p.TenantID, existing.ID, row.Name); rerr != nil {
					errs = append(errs, map[string]string{"ref": row.Ref, "error": apperr.AsError(rerr).Message})
					continue
				}
				renamed++
			default:
				unchanged++
			}
		}
		// Archive sync-owned nodes absent from the feed. Every page, not the
		// first one: an axis here already holds ~20k nodes.
		if lerr := u.eachScopeNode(ctx, p.TenantID,
			scopedomain.ScopeNodeFilter{Axis: axis},
			func(n scopedomain.ScopeNodeRecord) error {
				if n.ExternalRef != "" && !seen[n.ExternalRef] && !n.IsAxisRoot {
					if aerr := u.nodes.ArchiveScopeNode(ctx, p.TenantID, n.ID); aerr == nil {
						archived++
					}
				}
				return nil
			}); lerr != nil {
			return lerr
		}
		return nil
	}

	if dry {
		sentinel := apperr.E(apperr.KindInternal, "dry_run_rollback", "dry run")
		if err := u.tx.WithinTx(ctx, func(ctx context.Context) error {
			if rerr := run(ctx); rerr != nil {
				return rerr
			}
			return sentinel
		}); err != nil && apperr.AsError(err).Code != "dry_run_rollback" {
			return "", err
		}
	} else {
		if err := u.tx.WithinTx(ctx, run); err != nil {
			return "", err
		}
		u.emit(ctx, p, "scope.bulk_upsert", "", map[string]string{"axis": axis, "rows": itoaLen(rows)})
	}
	report["added"], report["renamed"], report["moved"] = added, renamed, moved
	report["archived"], report["unchanged"], report["errors"] = archived, unchanged, errs
	raw, _ := json.Marshal(report)
	return string(raw), nil
}

func (u *scopeAdminInteractor) ListSyncSources(ctx context.Context) ([]scopedomain.SyncSourceRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:sync:admin")
	if err != nil {
		return nil, err
	}
	return u.sync.ListSyncSources(ctx, p.TenantID)
}

func (u *scopeAdminInteractor) CreateSyncSource(ctx context.Context, s scopedomain.SyncSourceRecord) (*scopedomain.SyncSourceRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:sync:admin")
	if err != nil {
		return nil, err
	}
	if err := validateSyncConfig(s); err != nil {
		return nil, err
	}
	id, err := u.sync.CreateSyncSource(ctx, p.TenantID, s)
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "sync.source_create", id, map[string]string{"axis": s.Axis, "kind": s.Kind})
	return u.sync.SyncSource(ctx, p.TenantID, id)
}

// UpdateSyncSource rotates a feed's credentials or endpoint in place. The
// config is REPLACED wholesale — merging secrets is how half-rotated
// credentials happen.
func (u *scopeAdminInteractor) UpdateSyncSource(ctx context.Context, src scopedomain.SyncSourceRecord) (*scopedomain.SyncSourceRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:sync:admin")
	if err != nil {
		return nil, err
	}
	existing, err := u.sync.SyncSource(ctx, p.TenantID, src.ID)
	if err != nil {
		return nil, apperr.ErrNotFound
	}
	// Kind is immutable: it decides how config is interpreted, and a source
	// that changes kind is a different source.
	src.Kind = existing.Kind
	src.Axis = existing.Axis
	if err := validateSyncConfig(src); err != nil {
		return nil, err
	}
	if err := u.sync.UpdateSyncSource(ctx, p.TenantID, src); err != nil {
		return nil, err
	}
	// Never echo dsn/auth_header back into the audit detail.
	u.emit(ctx, p, "sync.source_update", src.ID, map[string]string{
		"axis": existing.Axis, "kind": existing.Kind,
	})
	return u.sync.SyncSource(ctx, p.TenantID, src.ID)
}

// SetSyncSchedule puts a source on a clock or takes it off one. It deliberately
// cannot touch config: the console is never sent dsn or auth_header, so a
// screen that rescheduled through UpdateSyncSource — which replaces config
// wholesale, on purpose — would save the source with its credentials gone.
func (u *scopeAdminInteractor) SetSyncSchedule(ctx context.Context, sourceID string, intervalSeconds int32) (*scopedomain.SyncSourceRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:sync:admin")
	if err != nil {
		return nil, err
	}
	if intervalSeconds != 0 && intervalSeconds < scopedomain.MinSyncIntervalSeconds {
		return nil, apperr.ErrInvalidArgument.With("interval_seconds",
			"0 for manual, or at least 300 seconds")
	}
	existing, err := u.sync.SyncSource(ctx, p.TenantID, sourceID)
	if err != nil {
		return nil, apperr.ErrNotFound
	}
	if err := u.sync.SetSyncSchedule(ctx, p.TenantID, sourceID, intervalSeconds); err != nil {
		return nil, err
	}
	u.emit(ctx, p, "sync.schedule_set", sourceID, map[string]string{
		"axis": existing.Axis, "interval_seconds": strconv.Itoa(int(intervalSeconds)),
	})
	return u.sync.SyncSource(ctx, p.TenantID, sourceID)
}

// RunSync reconciles a structure from its source of truth. Rows may be
// PUSHED by the caller, or — when none are supplied — PULLED by the server
// from wherever the structure actually lives: an HTTP API, a query against
// another database, or a table in another database, each over that source's
// OWN connection (migrations/0017; config carries the url/dsn). Either way
// the rows go through the database-side reconciler (scope_sync_apply) with
// its per-row error capture and dry-run support.
// maxSyncRuns bounds one read. History is for glancing at, and a feed that
// runs hourly still fits a year of interest inside this.
const maxSyncRuns = 100

func (u *scopeAdminInteractor) ListSyncRuns(ctx context.Context, sourceID string, limit int32) ([]scopedomain.SyncRun, error) {
	p, err := u.guard.Require(ctx, "anubis:sync:admin")
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxSyncRuns {
		limit = 25
	}
	return u.sync.ListSyncRuns(ctx, p.TenantID, sourceID, limit)
}

func (u *scopeAdminInteractor) RunSync(ctx context.Context, sourceID string, rows []SyncRowInput, dry bool) (string, error) {
	p, err := u.guard.Require(ctx, "anubis:sync:admin")
	if err != nil {
		return "", err
	}
	source, err := u.sync.SyncSource(ctx, p.TenantID, sourceID)
	if err != nil {
		return "", apperr.ErrNotFound
	}
	return u.runSync(ctx, p.TenantID, p, *source, rows, dry)
}

// dueSyncBatch bounds one tick. A tick that finds more sources than this
// leaves the rest for the next minute rather than holding the advisory lock
// while it works through an installation's whole backlog.
const dueSyncBatch = 50

// RunDue is the scheduler. It checks no permission and takes its tenant from
// each record rather than from a caller, because the authority for the run was
// settled when an operator configured the source. See
// ScopeSyncSchedulerUsecase for why this is kept off the transport.
func (u *scopeAdminInteractor) RunDue(ctx context.Context, now time.Time, limit int32) (int, error) {
	if limit <= 0 || limit > dueSyncBatch {
		limit = dueSyncBatch
	}
	due, err := u.sync.DueSyncSources(ctx, now, limit)
	if err != nil {
		return 0, err
	}
	ran := 0
	for _, src := range due {
		u.runDueOne(ctx, src)
		ran++
	}
	return ran, nil
}

// runDueOne is one scheduled attempt. Separate from the loop so the reschedule
// can be deferred: a source has to move on whether or not the run worked, or a
// feed that is down becomes a hot loop against somebody else's server — the
// next tick would find it due again a minute later, forever.
func (u *scopeAdminInteractor) runDueOne(ctx context.Context, src scopedomain.SyncSourceRecord) {
	defer func() {
		if serr := u.sync.ScheduleNextSyncSource(ctx, src.ID); serr != nil {
			// Failing to reschedule IS the hot loop, so it is an error, not a
			// warning: this source is now due on every tick until it is fixed.
			u.logger.Error("scope source not rescheduled",
				"source", src.ID, "axis", src.Axis, "error", serr)
		}
	}()
	// One bad feed must not stop the others. The failure is already durable —
	// a fetch that fails emits sync.fetch_failed into the tenant's own audit
	// trail — and the loop is the wrong place to give up on every other tenant.
	if _, err := u.runSync(ctx, src.TenantID, nil, src, nil, false); err != nil {
		u.logger.Warn("scope sync failed", "source", src.ID, "axis", src.Axis,
			"tenant", src.TenantID, "error", apperr.AsError(err).Code)
	}
}

// recordFailure writes the run row for an attempt that never reached the
// reconciler. scope_sync_apply opens its own row and catches per-row errors, so
// anything that gets as far as reconciling records itself; a fetch that fails
// never gets there. Unattended that was invisible — the Source pane showed a
// schedule, an empty history, and no hint that nothing had synced for days.
//
// Losing the row must not fail the caller: the run already failed, and its
// reason reaches the audit trail either way. It is still worth shouting about.
func (u *scopeAdminInteractor) recordFailure(ctx context.Context, sourceID, reason string) {
	if err := u.sync.RecordSyncFailure(ctx, sourceID, reason); err != nil {
		u.logger.Error("scope sync failure not recorded", "source", sourceID, "error", err)
	}
}

// runSync is one attempt with the authority question already settled: either
// the guard passed, or there was no operator to guard because a timer asked.
// tenantID is passed rather than read from p precisely so the scheduled path
// cannot fall back to an ambient tenant it does not have.
func (u *scopeAdminInteractor) runSync(ctx context.Context, tenantID string,
	p *authctx.Principal, src scopedomain.SyncSourceRecord,
	rows []SyncRowInput, dry bool,
) (string, error) {
	source := &src
	sourceID := src.ID
	if source.Status != "active" {
		return "", apperr.ErrInvalidArgument.With("source", "disabled")
	}
	// Rows with no parent_ref attach to the axis root, so it must exist
	// before the reconciler runs — syncing into a brand-new axis is the
	// normal case, not an edge case.
	if _, err := u.nodes.EnsureAxisRoot(ctx, tenantID, source.Axis); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		if u.fetcher == nil {
			return "", apperr.ErrInvalidArgument.With("rows", "no rows supplied and no feed fetcher configured")
		}
		fetched, ferr := u.fetcher.Fetch(ctx, *source)
		if ferr != nil {
			// A feed that cannot be reached must not silently archive every
			// node it was supposed to confirm.
			u.emitSync(ctx, tenantID, p, "sync.fetch_failed", sourceID, map[string]string{
				"kind": source.Kind, "error": apperr.AsError(ferr).Code,
			})
			u.recordFailure(ctx, sourceID, ferr.Error())
			return "", ferr
		}
		if len(fetched) == 0 {
			u.recordFailure(ctx, sourceID, "feed returned zero rows; refusing to archive the whole axis")
			return "", apperr.ErrInvalidArgument.With("feed", "returned zero rows; refusing to archive the whole axis")
		}
		rows = make([]SyncRowInput, 0, len(fetched))
		for _, f := range fetched {
			rows = append(rows, SyncRowInput{
				Ref: f.Ref, ParentRef: f.ParentRef, Name: f.Name, NodeType: f.NodeType,
			})
		}
	}
	if len(rows) > 50000 {
		return "", apperr.ErrInvalidArgument.With("rows", "max 50000 per run")
	}
	// Pushed rows get the same parents-first guarantee as pulled ones.
	feedRows := make([]scopedomain.SyncFeedRow, 0, len(rows))
	for _, r := range rows {
		feedRows = append(feedRows, scopedomain.SyncFeedRow{
			Ref: r.Ref, ParentRef: r.ParentRef, Name: r.Name, NodeType: r.NodeType,
		})
	}
	feedRows = scopedomain.SortFeedParentsFirst(feedRows)
	rows = rows[:0]
	for _, f := range feedRows {
		rows = append(rows, SyncRowInput{
			Ref: f.Ref, ParentRef: f.ParentRef, Name: f.Name, NodeType: f.NodeType,
		})
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return "", apperr.ErrInvalidArgument.Wrap(err)
	}
	report, err := u.sync.ScopeSyncApply(ctx, sourceID, raw, dry)
	if err != nil {
		return "", err
	}
	if !dry {
		u.emitSync(ctx, tenantID, p, "sync.run", sourceID, map[string]string{
			"rows": itoaLen(rows), "kind": source.Kind,
		})
	}
	return report, nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	s := slugRe.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-")
	if len(s) > 40 {
		s = s[:40]
	}
	if s == "" {
		s = "node"
	}
	return s
}

func suffix(ref string) string {
	if len(ref) > 6 {
		ref = ref[len(ref)-6:]
	}
	return strings.ToLower(ref)
}

func itoaLen[T any](v []T) string {
	n := len(v)
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// validateSyncConfig rejects a source whose config cannot possibly work, at
// creation rather than at 3am when the sync runs. Credentials inside dsn/
// auth_header are never logged or echoed back.
func validateSyncConfig(s scopedomain.SyncSourceRecord) error {
	// The schema carries the same floor as a CHECK. Repeating it here is what
	// turns a constraint violation into a message an operator can act on.
	if s.IntervalSeconds != 0 && s.IntervalSeconds < scopedomain.MinSyncIntervalSeconds {
		return apperr.ErrInvalidArgument.With("interval_seconds",
			"0 for manual, or at least 300 seconds")
	}
	var cfg map[string]any
	if err := json.Unmarshal(s.Config, &cfg); err != nil {
		return apperr.ErrInvalidArgument.With("config", "invalid JSON")
	}
	need := func(keys ...string) error {
		for _, k := range keys {
			if v, ok := cfg[k].(string); !ok || v == "" {
				return apperr.ErrInvalidArgument.With("config", "missing "+k)
			}
		}
		return nil
	}
	switch s.Kind {
	case "http":
		if err := need("url"); err != nil {
			return err
		}
		u, _ := cfg["url"].(string)
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return apperr.ErrInvalidArgument.With("url", "http(s) only")
		}
	case "db_query":
		return need("dsn", "query")
	case "db_table":
		if err := need("dsn", "table"); err != nil {
			return err
		}
		cols, ok := cfg["columns"].(map[string]any)
		if !ok {
			return apperr.ErrInvalidArgument.With("columns", "required for db_table")
		}
		for _, k := range []string{"ref", "name"} {
			if v, ok := cols[k].(string); !ok || v == "" {
				return apperr.ErrInvalidArgument.With("columns", "missing "+k)
			}
		}
	default:
		return apperr.ErrInvalidArgument.With("kind", s.Kind)
	}
	return nil
}

// ScopeNode fetches one node. Without it the console would have to pull a
// whole axis to display a single selected value.
func (u *scopeAdminInteractor) ScopeNode(ctx context.Context, id string) (*scopedomain.ScopeNodeRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	return u.nodes.ScopeNode(ctx, p.TenantID, id)
}

// maxNodeLookup bounds one batch. A screen shows tens of rows; anything
// asking for thousands is trying to page through the axis by another name,
// and ListScopeChildren / SearchScopeNodes are the calls for that.
const maxNodeLookup = 500

func (u *scopeAdminInteractor) ScopeNodes(ctx context.Context, ids []string) ([]scopedomain.ScopeNodeRecord, error) {
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > maxNodeLookup {
		return nil, apperr.ErrInvalidArgument.
			With("ids", "at most 500 per call").
			With("hint", "resolve the names for one page, not the whole axis")
	}
	return u.nodes.ScopeNodesByIDs(ctx, p.TenantID, ids)
}

// ScopeAncestors is the chain from the axis root down, which is what turns a
// scope decision into an explanation rather than a bare yes or no.
func (u *scopeAdminInteractor) ScopeAncestors(ctx context.Context, id string) ([]scopedomain.ScopeAncestor, error) {
	// The principal, not `_`. Require answers "may this caller read
	// identities?" and never "is this node theirs?" -- so discarding p left
	// the id taken on trust, and the answer is a chain of names, which is the
	// shape of whoever's organisation the id belonged to.
	p, err := u.guard.Require(ctx, "anubis:identity:read")
	if err != nil {
		return nil, err
	}
	return u.nodes.ScopeAncestors(ctx, p.TenantID, id)
}
