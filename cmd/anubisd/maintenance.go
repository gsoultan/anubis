package main

import (
	"context"
	"log/slog"
	"time"

	auditport "github.com/gsoultan/anubis/internal/audit/port"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	identityapp "github.com/gsoultan/anubis/internal/identity/app"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/jobs"
)

// Advisory lock ids for maintenance. Fixed and distinct so replicas contend
// per job rather than serialising all maintenance behind one lock.
const (
	lockPartitions = 0x616e7562_0001
	lockSweepOTT   = 0x616e7562_0002
	lockRetention  = 0x616e7562_0003
	lockKeyCheck   = 0x616e7562_0004
)

// maintenanceJobs is everything that must keep running for the database to
// stay healthy. Each is idempotent and safe to skip a tick.
func maintenanceJobs(
	partitions auditport.PartitionMaintainer,
	tokens authport.OneTimeSweeper,
	retention identityapp.RetentionUsecase,
	keys authport.KeyRepository,
	logger *slog.Logger,
) []jobs.Job {
	return []jobs.Job{
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
