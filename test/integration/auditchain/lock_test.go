//go:build integration

package auditchain

import (
	"context"
	"testing"

	"github.com/gsoultan/anubis/internal/platform/database"
)

// TryLock is what stops two replicas running the same maintenance job. It is
// SESSION-scoped, so it has to be taken and released on one pinned connection:
// released through the pool it would run on whichever connection came back
// next, release nothing, and leave the lock held until that connection was
// recycled — after which every replica would think the job was already running.
func TestTryLockIsExclusiveAndReleases(t *testing.T) {
	skipWithoutDB(t)
	ctx := context.Background()
	db := database.New(pool)
	const key = int64(7_449_777)

	acquired, release, err := db.TryLock(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("could not take a free lock")
	}

	// A second attempt must be refused while the first holds it — and must
	// NOT be an error: another replica having the job is the ordinary case.
	again, release2, err := db.TryLock(ctx, key)
	if err != nil {
		t.Fatalf("a contended lock reported an error: %v", err)
	}
	if again {
		release2()
		release()
		t.Fatal("the same lock was taken twice")
	}
	if release2 != nil {
		t.Fatal("a refused lock handed back a release function")
	}

	release()

	// And it is genuinely free afterwards, which is what proves the unlock ran
	// on the session that held it.
	third, release3, err := db.TryLock(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if !third {
		t.Fatal("the lock was not released")
	}
	release3()
}
