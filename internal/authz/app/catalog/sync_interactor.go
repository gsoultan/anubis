package authzcatalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	"github.com/gsoultan/anubis/internal/authz/domain/catalogsync"
	"github.com/gsoultan/anubis/internal/authz/guard"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// Configuring a source is the privileged act, not running one: the URL
// decides what the catalog will say. So it takes the same permission as
// applying a manifest by hand, and nothing weaker.
const permApply = "anubis:manifest:apply"

// A scheduler tick runs at most this many sources. A tick that tried to run
// every due source in one pass would hold the advisory lock for as long as
// the slowest feed in the installation.
const dueBatch = 25

type syncInteractor struct {
	guard   *guard.Guard
	repo    authzport.CatalogSyncRepository
	fetch   authzport.CatalogFetcher
	apply   CatalogApplier
	apps    tenancyport.ApplicationRepository
	audit   auditport.Auditor
	logger  *slog.Logger
	nowFunc func() time.Time
}

func NewSyncInteractor(
	ops guard.OperatorAuthority,
	clockNow func() time.Time,
	repo authzport.CatalogSyncRepository,
	fetch authzport.CatalogFetcher,
	apply CatalogApplier,
	apps tenancyport.ApplicationRepository,
	audit auditport.Auditor,
	logger *slog.Logger,
) CatalogSyncUsecase {
	return &syncInteractor{
		guard: guard.New().WithOperators(ops, clockNow), repo: repo, fetch: fetch,
		apply: apply, apps: apps, audit: audit, logger: logger, nowFunc: clockNow,
	}
}

func (u *syncInteractor) ListSources(ctx context.Context) ([]catalogsync.Source, error) {
	p, err := u.guard.Require(ctx, permApply)
	if err != nil {
		return nil, err
	}
	return u.repo.ListSources(ctx, p.TenantID)
}

func (u *syncInteractor) ListRuns(ctx context.Context, sourceID string, limit int32) ([]catalogsync.Run, error) {
	p, err := u.guard.Require(ctx, permApply)
	if err != nil {
		return nil, err
	}
	// Read the source first: it is the only thing that binds a run history to
	// a tenant, and a history is a list of what changed in an access catalog.
	if _, err := u.repo.Source(ctx, p.TenantID, sourceID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return u.repo.ListRuns(ctx, sourceID, limit)
}

func (u *syncInteractor) CreateSource(ctx context.Context, in SourceInput) (*catalogsync.Source, error) {
	p, err := u.guard.Require(ctx, permApply)
	if err != nil {
		return nil, err
	}
	src, err := u.validate(ctx, p.TenantID, in)
	if err != nil {
		return nil, err
	}
	id, err := u.repo.CreateSource(ctx, *src)
	if err != nil {
		return nil, err
	}
	u.emit(ctx, p, "catalog.source.create", id, map[string]string{
		"application": in.ApplicationSlug, "kind": src.Kind, "format": src.Format,
		"interval_seconds": strconv.Itoa(src.IntervalSeconds),
	})
	return u.repo.Source(ctx, p.TenantID, id)
}

func (u *syncInteractor) UpdateSource(ctx context.Context, in SourceInput) (*catalogsync.Source, error) {
	p, err := u.guard.Require(ctx, permApply)
	if err != nil {
		return nil, err
	}
	if in.ID == "" {
		return nil, apperr.ErrInvalidArgument.With("id", "required")
	}
	// The application a source writes to is fixed at creation. Repointing it
	// would turn one team's feed into another team's catalog without anybody
	// re-approving the URL; delete the source and make a new one instead.
	existing, err := u.repo.Source(ctx, p.TenantID, in.ID)
	if err != nil {
		return nil, err
	}
	in.ApplicationSlug = existing.ApplicationSlug
	src, err := u.validate(ctx, p.TenantID, in)
	if err != nil {
		return nil, err
	}
	src.ID = in.ID
	if err := u.repo.UpdateSource(ctx, *src); err != nil {
		return nil, err
	}
	u.emit(ctx, p, "catalog.source.update", in.ID, map[string]string{
		"application": src.ApplicationSlug, "status": src.Status,
		"interval_seconds": strconv.Itoa(src.IntervalSeconds),
	})
	return u.repo.Source(ctx, p.TenantID, in.ID)
}

func (u *syncInteractor) DeleteSource(ctx context.Context, sourceID string) error {
	p, err := u.guard.Require(ctx, permApply)
	if err != nil {
		return err
	}
	// Read it first: the audit entry has to name what was removed, and the
	// read is what proves it belonged to this tenant.
	src, err := u.repo.Source(ctx, p.TenantID, sourceID)
	if err != nil {
		return err
	}
	if err := u.repo.DeleteSource(ctx, p.TenantID, sourceID); err != nil {
		return err
	}
	u.emit(ctx, p, "catalog.source.delete", sourceID, map[string]string{
		"application": src.ApplicationSlug, "name": src.Name,
	})
	return nil
}

func (u *syncInteractor) RunSource(ctx context.Context, sourceID string, dry bool) (*catalogsync.Run, error) {
	p, err := u.guard.Require(ctx, permApply)
	if err != nil {
		return nil, err
	}
	src, err := u.repo.Source(ctx, p.TenantID, sourceID)
	if err != nil {
		return nil, err
	}
	if src.Status != catalogsync.StatusActive {
		return nil, apperr.ErrInvalidArgument.With("source", "disabled")
	}
	actor := p.IdentityID
	if actor == "" {
		actor = catalogsync.ActorSystem
	}
	return u.run(ctx, *src, actor, dry)
}

// RunDue is the scheduler. It takes no principal and checks no permission,
// because there is nobody to check: the authority for this run was settled
// when an operator configured the source.
func (u *syncInteractor) RunDue(ctx context.Context, now time.Time, limit int32) (int, error) {
	if limit <= 0 || limit > dueBatch {
		limit = dueBatch
	}
	due, err := u.repo.DueSources(ctx, now, limit)
	if err != nil {
		return 0, err
	}
	ran := 0
	for _, src := range due {
		// One bad feed must not stop the others: a source that fails has
		// already recorded WHY in its own run history, and the loop is the
		// wrong place to give up on every other tenant.
		if _, rerr := u.run(ctx, src, catalogsync.ActorSystem, false); rerr != nil {
			u.logger.Warn("catalog sync failed",
				"source", src.ID, "application", src.ApplicationSlug,
				"error", apperr.AsError(rerr).Code)
		}
		ran++
	}
	return ran, nil
}

// run is one attempt, and its whole job is that the attempt is recorded
// whatever happens to it. The run row is opened BEFORE the fetch and closed
// outside the apply's transaction — an apply that fails rolls its own rows
// back, and the evidence that it was tried must not roll back with them.
func (u *syncInteractor) run(ctx context.Context, src catalogsync.Source, actor string, dry bool) (*catalogsync.Run, error) {
	runID, err := u.repo.StartRun(ctx, src.ID, src.TenantID, actor, dry)
	if err != nil {
		return nil, err
	}
	finish := func(status, digest, report, errMsg string) {
		if ferr := u.repo.FinishRun(ctx, runID, status, digest, report, errMsg); ferr != nil {
			// The apply itself already happened; losing the history entry is
			// not a reason to fail the caller, but it IS a reason to shout.
			u.logger.Error("catalog run history lost", "run", runID, "error", ferr)
		}
	}
	// A scheduled source moves on even when the run failed, or a feed that is
	// down becomes a hot loop against somebody else's server.
	defer func() {
		if src.Scheduled() && !dry {
			if serr := u.repo.ScheduleNext(ctx, src.ID); serr != nil {
				u.logger.Error("catalog source not rescheduled", "source", src.ID, "error", serr)
			}
		}
	}()

	document, err := u.fetch.Fetch(ctx, src)
	if err != nil {
		finish(catalogsync.RunFailed, "", "", apperr.AsError(err).Error())
		return nil, err
	}
	sum := sha256.Sum256([]byte(document))
	digest := hex.EncodeToString(sum[:])

	if !dry {
		last, lerr := u.repo.LastAppliedDigest(ctx, src.ID)
		if lerr == nil && last == digest {
			// Nothing changed, so nothing is written and no manifest version
			// is burned — a version bump is what forces every gate to rebuild
			// its snapshot (migration 0040).
			finish(catalogsync.RunSkipped, digest, `{"skipped":"document unchanged"}`, "")
			return u.lastRun(ctx, src.ID)
		}
	}

	report, version, err := u.apply.ApplyDocumentAsSystem(ctx, src.TenantID,
		src.ApplicationSlug, document, src.Format, dry)
	if err != nil {
		finish(catalogsync.RunFailed, digest, "", apperr.AsError(err).Error())
		return nil, err
	}
	status := catalogsync.RunOK
	if dry {
		status = catalogsync.RunDry
	}
	finish(status, digest, enrich(report, version), "")
	return u.lastRun(ctx, src.ID)
}

// enrich folds the manifest version into the stored report so a history
// entry answers "which version did this produce" without a second lookup.
func enrich(report string, version int) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(report), &m); err != nil || m == nil {
		return report
	}
	m["manifest_version"] = version
	out, err := json.Marshal(m)
	if err != nil {
		return report
	}
	return string(out)
}

func (u *syncInteractor) lastRun(ctx context.Context, sourceID string) (*catalogsync.Run, error) {
	runs, err := u.repo.ListRuns(ctx, sourceID, 1)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return &runs[0], nil
}

// validate turns what an operator typed into a source, refusing the values
// that would otherwise fail deep inside a fetch or a constraint.
func (u *syncInteractor) validate(ctx context.Context, tenantID string, in SourceInput) (*catalogsync.Source, error) {
	app, err := u.apps.ApplicationBySlug(ctx, tenantID, in.ApplicationSlug)
	if err != nil {
		return nil, apperr.ErrNotFound.With("application", in.ApplicationSlug)
	}
	if in.Name == "" {
		return nil, apperr.ErrInvalidArgument.With("name", "required")
	}
	if in.Kind == "" {
		in.Kind = catalogsync.KindHTTP
	}
	if in.Kind != catalogsync.KindHTTP {
		return nil, apperr.ErrInvalidArgument.With("kind", in.Kind).With("expected", catalogsync.KindHTTP)
	}
	switch in.Format {
	case FormatJSON, FormatCSV:
	case "":
		in.Format = FormatJSON
	default:
		return nil, apperr.ErrInvalidArgument.With("format", in.Format).
			With("expected", FormatJSON+" or "+FormatCSV)
	}
	switch in.Status {
	case catalogsync.StatusActive, catalogsync.StatusDisabled:
	case "":
		in.Status = catalogsync.StatusActive
	default:
		return nil, apperr.ErrInvalidArgument.With("status", in.Status)
	}
	if !json.Valid([]byte(in.ConfigJSON)) {
		return nil, apperr.ErrInvalidArgument.With("config", "not JSON")
	}
	if in.IntervalSeconds != 0 && in.IntervalSeconds < 300 {
		return nil, apperr.ErrInvalidArgument.
			With("interval_seconds", strconv.Itoa(in.IntervalSeconds)).
			With("minimum", "300 — a catalog is a document, not a data feed")
	}
	return &catalogsync.Source{
		TenantID: tenantID, ApplicationID: app.ID, ApplicationSlug: app.Slug,
		Kind: in.Kind, Format: in.Format, Status: in.Status, Name: in.Name,
		Config: []byte(in.ConfigJSON), IntervalSeconds: in.IntervalSeconds,
	}, nil
}

func (u *syncInteractor) emit(ctx context.Context, p *authctx.Principal, action, target string, detail map[string]string) {
	u.audit.Emit(ctx, auditdomain.AuditEvent{
		TenantID: p.TenantID, ActorID: p.IdentityID, ActorKind: "identity",
		SessionID: p.SessionID, TargetID: target, Action: action, Result: "allow",
		IP: authctx.ClientIP(ctx), Detail: jsonx.Must(detail),
	})
}
