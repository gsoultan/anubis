package scopeapp

import (
	"context"
	"fmt"
	"testing"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	scopeport "github.com/gsoultan/anubis/internal/scope/port"
)

// pagedNodes serves canned pages and records the filters it was asked for.
// The embedded nil interface satisfies the rest of ScopeNodeRepository; any
// method this test does not exercise panics rather than quietly returning
// zero values.
type pagedNodes struct {
	scopeport.ScopeNodeRepository
	total   int
	seen    []scopedomain.ScopeNodeFilter
	perPage int32
}

func (p *pagedNodes) ListScopeNodes(_ context.Context, _ string, f scopedomain.ScopeNodeFilter) ([]scopedomain.ScopeNodeRecord, error) {
	f = f.Normalise()
	if p.perPage > 0 && f.Limit > p.perPage {
		f.Limit = p.perPage
	}
	p.seen = append(p.seen, f)

	start := 0
	if f.AfterID != "" {
		if _, err := fmt.Sscanf(f.AfterID, "n%d", &start); err != nil {
			return nil, err
		}
		start++
	}
	var out []scopedomain.ScopeNodeRecord
	for i := start; i < p.total && int32(len(out)) < f.Limit; i++ {
		out = append(out, scopedomain.ScopeNodeRecord{
			ID: fmt.Sprintf("n%d", i), Name: fmt.Sprintf("%06d", i), Axis: f.Axis,
		})
	}
	return out, nil
}

func walkAll(t *testing.T, total int, perPage int32) ([]string, *pagedNodes) {
	t.Helper()
	repo := &pagedNodes{total: total, perPage: perPage}
	u := &scopeAdminInteractor{nodes: repo}
	var got []string
	err := u.eachScopeNode(context.Background(), "tenant",
		scopedomain.ScopeNodeFilter{Axis: "org", Limit: perPage},
		func(n scopedomain.ScopeNodeRecord) error {
			got = append(got, n.ID)
			return nil
		})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return got, repo
}

// The archive pass in UpsertScopeNodes decides what the feed no longer
// contains from this walk. Miss a page and sync-owned nodes that were deleted
// upstream stay active — along with every grant scoped to them.
func TestEachScopeNodeVisitsEveryNode(t *testing.T) {
	for _, total := range []int{0, 1, 9, 10, 11, 250} {
		got, _ := walkAll(t, total, 10)
		if len(got) != total {
			t.Errorf("total=%d: walked %d nodes", total, len(got))
			continue
		}
		for i, id := range got {
			if want := fmt.Sprintf("n%d", i); id != want {
				t.Errorf("total=%d: node %d = %s, want %s", total, i, id, want)
				break
			}
		}
	}
}

// An exact multiple of the page size is the off-by-one: the last full page
// looks identical to a page with more behind it, so the walk must ask once
// more and only stop on a genuinely short page.
func TestEachScopeNodeHandlesExactPageMultiples(t *testing.T) {
	got, repo := walkAll(t, 30, 10)
	if len(got) != 30 {
		t.Fatalf("walked %d nodes, want 30", len(got))
	}
	if len(repo.seen) != 4 {
		t.Errorf("made %d queries, want 4 (3 full pages + one short)", len(repo.seen))
	}
}

func TestEachScopeNodeAdvancesTheCursor(t *testing.T) {
	_, repo := walkAll(t, 25, 10)
	if len(repo.seen) < 2 {
		t.Fatal("only one page was requested")
	}
	if repo.seen[0].AfterID != "" {
		t.Errorf("first query resumed from %q, want the beginning", repo.seen[0].AfterID)
	}
	for i, f := range repo.seen[1:] {
		if f.AfterID == "" {
			t.Errorf("query %d restarted from the beginning — this would loop forever", i+1)
		}
		if f.Axis != "org" {
			t.Errorf("query %d lost the axis filter", i+1)
		}
	}
}

// A caller that stops early must stop the walk, not keep paging.
func TestEachScopeNodeStopsOnError(t *testing.T) {
	repo := &pagedNodes{total: 1000, perPage: 10}
	u := &scopeAdminInteractor{nodes: repo}
	sentinel := fmt.Errorf("stop")
	err := u.eachScopeNode(context.Background(), "tenant",
		scopedomain.ScopeNodeFilter{Axis: "org", Limit: 10},
		func(scopedomain.ScopeNodeRecord) error { return sentinel })
	if err != sentinel {
		t.Fatalf("err = %v, want the caller's error", err)
	}
	if len(repo.seen) != 1 {
		t.Errorf("kept paging after the callback failed (%d queries)", len(repo.seen))
	}
}
