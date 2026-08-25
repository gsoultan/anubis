// Package jobs runs the recurring maintenance an auth database needs to stay
// healthy: partition provisioning, single-use token sweeping, retention
// enforcement, key-expiry warnings.
//
// Every job runs under a Postgres advisory lock, so N replicas of anubisd
// cooperate without a leader election: whoever gets the lock does the work,
// the rest skip that tick. A job that cannot take the lock is not an error.
package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/gsoultan/anubis/internal/platform/metrics"
)

// Job is one unit of recurring maintenance.
type Job struct {
	Name string
	// Every is the interval between attempts.
	Every time.Duration
	// LockID identifies the advisory lock; distinct per job.
	LockID int64
	// Run does the work. It receives a context already bounded by Timeout.
	Run func(ctx context.Context) error
	// Timeout bounds a single execution (default 5 minutes).
	Timeout time.Duration
	// RunAtStart executes once immediately instead of waiting a full interval
	// — partitions must exist before the first insert, not an hour later.
	RunAtStart bool
}

// Locker takes and releases session-scoped advisory locks.
type Locker interface {
	TryLock(ctx context.Context, id int64) (acquired bool, release func(), err error)
}

// Scheduler runs jobs until its context ends.
type Scheduler struct {
	locker Locker
	logger *slog.Logger
	jobs   []Job
}

func NewScheduler(locker Locker, logger *slog.Logger, jobs ...Job) *Scheduler {
	return &Scheduler{locker: locker, logger: logger, jobs: jobs}
}

// Run blocks until ctx is done, then returns once every job goroutine has
// stopped.
func (s *Scheduler) Run(ctx context.Context) {
	done := make(chan struct{}, len(s.jobs))
	for _, j := range s.jobs {
		go func(j Job) {
			defer func() { done <- struct{}{} }()
			s.loop(ctx, j)
		}(j)
	}
	for range s.jobs {
		<-done
	}
}

func (s *Scheduler) loop(ctx context.Context, j Job) {
	if j.Timeout <= 0 {
		j.Timeout = 5 * time.Minute
	}
	if j.RunAtStart {
		s.once(ctx, j)
	}
	t := time.NewTicker(j.Every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.once(ctx, j)
		}
	}
}

func (s *Scheduler) once(ctx context.Context, j Job) {
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("job panicked", "job", j.Name, "panic", rec)
		}
	}()
	runCtx, cancel := context.WithTimeout(ctx, j.Timeout)
	defer cancel()

	acquired, release, err := s.locker.TryLock(runCtx, j.LockID)
	if err != nil {
		s.logger.Warn("job lock failed", "job", j.Name, "error", err)
		metrics.IncJob(j.Name, "error")
		return
	}
	if !acquired {
		metrics.IncJob(j.Name, "skipped") // another replica is doing it; not an error
		return
	}
	defer release()

	start := time.Now()
	if err := j.Run(runCtx); err != nil {
		s.logger.Error("job failed", "job", j.Name,
			"duration_ms", time.Since(start).Milliseconds(), "error", err)
		metrics.IncJob(j.Name, "error")
		return
	}
	s.logger.Info("job ok", "job", j.Name, "duration_ms", time.Since(start).Milliseconds())
	metrics.IncJob(j.Name, "ok")
}
