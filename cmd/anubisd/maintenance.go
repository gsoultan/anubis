package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	auditpg "github.com/gsoultan/anubis/internal/audit/adapter/postgres"
	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	authzcatalog "github.com/gsoultan/anubis/internal/authz/app/catalog"
	controlport "github.com/gsoultan/anubis/internal/control/port"
	identityapp "github.com/gsoultan/anubis/internal/identity/app"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/jobs"
	scopeapp "github.com/gsoultan/anubis/internal/scope/app"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// Advisory lock ids for maintenance. Fixed and distinct so replicas contend
// per job rather than serialising all maintenance behind one lock.
const (
	lockPartitions   = 0x616e7562_0001
	lockSweepOTT     = 0x616e7562_0002
	lockRetention    = 0x616e7562_0003
	lockKeyCheck     = 0x616e7562_0004
	lockSweepRefresh = 0x616e7562_0005
	lockCatalogSync  = 0x616e7562_0006
	lockScopeSync    = 0x616e7562_0007
	lockAuditAnchor  = 0x616e7562_0008
)

// maintenanceJobs is everything that must keep running for the database to
// stay healthy. Each is idempotent and safe to skip a tick.
func maintenanceJobs(
	partitions auditport.PartitionMaintainer,
	tokens authport.OneTimeSweeper,
	retention identityapp.RetentionUsecase,
	keys authport.KeyRepository,
	refresh controlport.PlatformRefreshStore,
	catalog authzcatalog.CatalogSyncUsecase,
	scope scopeapp.ScopeSyncSchedulerUsecase,
	anchors *auditpg.ChainedAuditor,
	ring *keyring.Manager,
	tenants tenancyport.TenantRepository,
	logger *slog.Logger,
) []jobs.Job {
	return []jobs.Job{
		{
			// Sign each tenant's chain head. The chain proves it is
			// self-consistent; a rewrite is self-consistent too, so what
			// makes tampering detectable is a hash somebody signed at the
			// time with a key sealed under the master.
			//
			// Hourly, not continuous: an anchor bounds how much history a
			// rewrite could reach without being caught, and an hour of
			// exposure is the trade for not signing on every write.
			Name: "audit_anchor", Every: time.Hour, LockID: lockAuditAnchor,
			Timeout: 5 * time.Minute,
			Run: func(ctx context.Context) error {
				list, err := tenants.ListTenants(ctx)
				if err != nil {
					return err
				}
				var failed int
				for _, tn := range list {
					if err := anchors.AnchorChain(ctx, ring, tn.ID); err != nil {
						// One tenant's failure must not stop the rest: an
						// un-anchored chain is the thing this job exists to
						// prevent, so the others still need theirs.
						failed++
						logger.Error("could not anchor audit chain",
							"tenant", tn.ID, "error", err)
					}
				}
				if failed > 0 {
					return fmt.Errorf("%d of %d tenants could not be anchored", failed, len(list))
				}
				return nil
			},
		},
		{
			// Structures that have come due. Same shape as catalog_sync, on
			// purpose: a minute is the TICK, each source carries its own
			// interval with a five-minute floor (0045), and a tick that finds
			// nothing is one probe of the partial index over scheduled
			// sources alone.
			//
			// Its own lock, not catalog_sync's. Sharing one would serialise
			// every tenant's structure refresh behind every tenant's catalog
			// refresh, and a slow ERP would starve the other job entirely.
			//
			// The timeout is generous because a feed is somebody else's
			// server: an axis in the benchmark dataset holds ~20,000 nodes,
			// and the reconcile is one statement per row.
			Name: "scope_sync", Every: time.Minute, LockID: lockScopeSync,
			Timeout: 10 * time.Minute,
			Run: func(ctx context.Context) error {
				n, err := scope.RunDue(ctx, time.Now(), 0)
				if err == nil && n > 0 {
					logger.Info("scope sources run", "sources", n)
				}
				return err
			},
		},
		{
			// Catalog sources that have come due. A minute is the TICK, not
			// the interval — each source carries its own, with a five-minute
			// floor — and a tick that finds nothing is one indexed query
			// against a partial index of the scheduled sources alone.
			//
			// The advisory lock is doing real work here, unlike in a sweep:
			// two replicas applying the same catalog at the same moment would
			// both bump the manifest version and both rebuild every gate.
			Name: "catalog_sync", Every: time.Minute, LockID: lockCatalogSync,
			Timeout: 10 * time.Minute,
			Run: func(ctx context.Context) error {
				n, err := catalog.RunDue(ctx, time.Now(), 0)
				if err == nil && n > 0 {
					logger.Info("catalog sources run", "sources", n)
				}
				return err
			},
		},
		{
			// Partitions must exist BEFORE the insert that needs them. The
			// DEFAULT partition catches a missed run, but rows landing there
			// defeat the point of partitioning (retention becomes a DELETE
			// again), so this runs at boot and daily.
			Name: "partitions", Every: 24 * time.Hour, LockID: lockPartitions,
			RunAtStart: true,
			Run:        partitions.EnsurePartitions,
		},
		{
			// Single-use tokens live seconds to minutes; expired rows are
			// pure bloat on a hot path.
			Name: "sweep_one_time_tokens", Every: time.Hour, LockID: lockSweepOTT,
			Run: func(ctx context.Context) error {
				n, err := tokens.SweepExpired(ctx)
				if err == nil && n > 0 {
					logger.Info("swept expired one-time tokens", "rows", n)
				}
				return err
			},
		},
		{
			// Operator refresh chains (0031) expire in hours; the rows only
			// matter to theft detection while their family is alive.
			Name: "sweep_platform_refresh", Every: 6 * time.Hour, LockID: lockSweepRefresh,
			Run: func(ctx context.Context) error {
				n, err := refresh.SweepExpired(ctx)
				if err == nil && n > 0 {
					logger.Info("swept expired platform refresh tokens", "rows", n)
				}
				return err
			},
		},
		{
			// Statutory retention. Anonymise + shred; see ADR-0007 and
			// migrations/0022.
			Name: "retention", Every: 6 * time.Hour, LockID: lockRetention,
			Timeout: 15 * time.Minute,
			Run: func(ctx context.Context) error {
				rep, err := retention.Sweep(ctx)
				if err == nil && (rep.Anonymized > 0 || rep.Stamped > 0) {
					logger.Info("retention sweep", "stamped", rep.Stamped,
						"anonymized", rep.Anonymized, "shredded", rep.Shredded)
				}
				return err
			},
		},
		{
			// Nobody notices an expiring signing key until tokens stop
			// verifying. Warn early and loudly; rotation stays a human
			// decision (anubisd keys prepare/promote).
			Name: "signing_key_expiry", Every: 6 * time.Hour, LockID: lockKeyCheck,
			RunAtStart: true,
			Run: func(ctx context.Context) error {
				records, err := keys.SigningKeys(ctx)
				if err != nil {
					return err
				}
				now := time.Now()
				for _, k := range records {
					if k.Status != keyring.StatusActive {
						continue
					}
					left := k.NotAfter.Sub(now)
					switch {
					case left <= 0:
						logger.Error("ACTIVE SIGNING KEY HAS EXPIRED — rotate now",
							"kid", k.Kid, "purpose", k.Purpose, "not_after", k.NotAfter)
					case left < 14*24*time.Hour:
						logger.Warn("signing key expires soon — run `anubisd keys prepare`",
							"kid", k.Kid, "purpose", k.Purpose, "days_left", int(left.Hours()/24))
					}
				}
				return nil
			},
		},
	}
}
