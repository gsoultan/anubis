package tenancycache

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

type movingClock struct{ t time.Time }

func (c *movingClock) Now() time.Time { return c.t }

type countingTenants struct {
	tenancyport.TenantRepository
	calls  int
	status string
}

func (r *countingTenants) TenantBySlug(_ context.Context, slug string) (*tenancydomain.TenantRef, error) {
	r.calls++
	if slug == "missing" {
		return nil, errors.New("not found")
	}
	return &tenancydomain.TenantRef{ID: "id-" + slug, Slug: slug, Status: r.status}, nil
}

func TestTheCacheAnswersFromMemoryUntilTheWindowCloses(t *testing.T) {
	clk := &movingClock{t: time.Unix(1_700_000_000, 0)}
	repo := &countingTenants{status: "active"}
	c := New(repo, clk, 5*time.Second, 16)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if ref, err := c.TenantBySlug(ctx, "acme"); err != nil || ref.ID != "id-acme" {
			t.Fatalf("lookup %d: %+v %v", i, ref, err)
		}
	}
	if repo.calls != 1 {
		t.Fatalf("three lookups inside the window reached the database %d times", repo.calls)
	}

	// The tenant is suspended; the window bounds how long that takes to show.
	repo.status = "suspended"
	clk.t = clk.t.Add(5 * time.Second)
	ref, _ := c.TenantBySlug(ctx, "acme")
	if repo.calls != 2 || ref.Serving() {
		t.Fatalf("after the window: calls=%d serving=%v, want a fresh read that is not serving", repo.calls, ref.Serving())
	}
}

func TestAMissIsNeverRemembered(t *testing.T) {
	repo := &countingTenants{status: "active"}
	c := New(repo, &movingClock{t: time.Unix(1_700_000_000, 0)}, time.Minute, 16)
	for i := 0; i < 3; i++ {
		if _, err := c.TenantBySlug(context.Background(), "missing"); err == nil {
			t.Fatal("a missing tenant was found")
		}
	}
	if repo.calls != 3 || len(c.entries) != 0 {
		t.Fatalf("misses: calls=%d entries=%d, want 3 and 0", repo.calls, len(c.entries))
	}
}

func TestTheCacheNeverHoldsMoreThanItsBound(t *testing.T) {
	c := New(&countingTenants{status: "active"}, &movingClock{t: time.Unix(1_700_000_000, 0)}, time.Minute, 8)
	for i := 0; i < 100; i++ {
		if _, err := c.TenantBySlug(context.Background(), fmt.Sprintf("t%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(c.entries); n > 8 {
		t.Fatalf("holds %d entries, bound is 8", n)
	}
}

func TestACallerCannotEditTheCachedTenant(t *testing.T) {
	c := New(&countingTenants{status: "active"}, &movingClock{t: time.Unix(1_700_000_000, 0)}, time.Minute, 8)
	ref, _ := c.TenantBySlug(context.Background(), "acme")
	ref.Status = "active-forever"
	again, _ := c.TenantBySlug(context.Background(), "acme")
	if again.Status != "active" {
		t.Fatalf("a caller's edit reached the cache: %q", again.Status)
	}
}

func TestAZeroWindowDisablesTheCache(t *testing.T) {
	repo := &countingTenants{status: "active"}
	c := New(repo, &movingClock{t: time.Unix(1_700_000_000, 0)}, 0, 8)
	_, _ = c.TenantBySlug(context.Background(), "acme")
	_, _ = c.TenantBySlug(context.Background(), "acme")
	if repo.calls != 2 {
		t.Fatalf("calls=%d, want every lookup to reach the database", repo.calls)
	}
}
