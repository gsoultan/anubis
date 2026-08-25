//go:build integration

package e2e

import (
	"context"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
)

/* The decision path under CONCURRENCY.

   authorize() itself is benchmarked in the database (0.045 ms) and through
   pgx (TestAuthorizeLatencyBudget, p95 < 2 ms), but both measure one caller
   at a time. Everything between — the interceptor, the guard, the endpoint
   middleware, the pgx pool — had never been measured under load at all, and
   those are exactly the layers where a per-request allocation, a lock held
   too long, or a pool sized wrong stops being invisible.

   Bounded so it belongs in CI: a few seconds, a few thousand decisions.
   Turn it up for a soak with ANUBIS_LOAD_WORKERS / ANUBIS_LOAD_SECONDS. */

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

func TestAuthorizeUnderConcurrency(t *testing.T) {
	requireServer(t)
	if testing.Short() {
		t.Skip("load")
	}
	ctx := context.Background()
	subject, permission := probeSubject(t)
	tokens := login(t)

	workers := envInt("ANUBIS_LOAD_WORKERS", 32)
	seconds := envInt("ANUBIS_LOAD_SECONDS", 3)
	// p99 rather than a mean: the tail is what a caller actually waits for,
	// and a mean hides a pool that is one connection short.
	budget := 50 * time.Millisecond

	az := anubisv1connect.NewAuthzServiceClient(http.DefaultClient, baseURL)

	// Warm the pool and the connection pool in the client. Without this the
	// first samples measure a cold start, which is real but is not what a
	// steady-state budget is asking about.
	for i := 0; i < workers; i++ {
		if _, err := az.Authorize(ctx, bearer(connect.NewRequest(&anubisv1.AuthorizeRequest{
			Subject: subject, Permission: permission,
		}), tokens.AccessToken)); err != nil {
			t.Fatalf("warmup: %v", err)
		}
	}

	deadline := time.Now().Add(time.Duration(seconds) * time.Second)

	var (
		mu      sync.Mutex
		lat     []time.Duration
		errored atomic.Int64
		denied  atomic.Int64
	)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]time.Duration, 0, 512)
			for time.Now().Before(deadline) {
				start := time.Now()
				resp, err := az.Authorize(ctx, bearer(connect.NewRequest(&anubisv1.AuthorizeRequest{
					Subject: subject, Permission: permission,
				}), tokens.AccessToken))
				elapsed := time.Since(start)
				switch {
				case err != nil:
					errored.Add(1)
				case !resp.Msg.Allow:
					// A deny here is a correctness failure, not a slow path:
					// the probe grant makes this decision an allow.
					denied.Add(1)
				}
				local = append(local, elapsed)
			}
			mu.Lock()
			lat = append(lat, local...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(lat) == 0 {
		t.Fatal("no requests completed")
	}
	if n := errored.Load(); n > 0 {
		t.Fatalf("%d of %d requests errored under %d-way concurrency", n, len(lat), workers)
	}
	if n := denied.Load(); n > 0 {
		t.Fatalf("%d decisions flipped to deny under load — the answer must not "+
			"depend on how busy the server is", n)
	}

	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	p := func(q float64) time.Duration { return lat[int(float64(len(lat)-1)*q)] }
	rps := float64(len(lat)) / float64(seconds)
	t.Logf("%d decisions in %ds across %d workers — %.0f/s, p50 %v, p95 %v, p99 %v, max %v",
		len(lat), seconds, workers, rps, p(0.50), p(0.95), p(0.99), lat[len(lat)-1])

	if got := p(0.99); got > budget {
		t.Fatalf("p99 %v over budget %v at %d-way concurrency (p50 %v) — the "+
			"tail is what a caller waits for", got, budget, workers, p(0.50))
	}
}

// probeSubject provisions a real permission and a grant of it, so the load
// runs against an ALLOW: a deny can short-circuit before the engine and
// would measure the wrong thing. Idempotent, like the rest of the suite.
func probeSubject(t *testing.T) (subject, permission string) {
	t.Helper()
	ctx := context.Background()
	opToken := platformLogin(t)

	if _, err := pageClient().CreateApplication(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateApplicationRequest{
		Application: &anubisv1.Application{Slug: "e2e-authz", Name: "e2e authz probe", Kind: "service"},
	}), opToken)); err != nil && connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("create application: %v", err)
	}
	aza := anubisv1connect.NewAuthzAdminServiceClient(http.DefaultClient, baseURL)
	if _, err := aza.ApplyManifest(ctx, operatorBearer(connect.NewRequest(&anubisv1.ApplyManifestRequest{
		ApplicationSlug: "e2e-authz",
		ManifestJson: `{"permissions":[{"resource":"probe","action":"read","description":"e2e probe"}],
		                "roles":[{"name":"reader","description":"e2e reader","permissions":["probe:read"]}]}`,
	}), opToken)); err != nil {
		t.Fatalf("apply manifest: %v", err)
	}
	roles, err := aza.ListRoles(ctx, operatorBearer(connect.NewRequest(&anubisv1.ListRolesRequest{
		Query: "e2e-authz.reader",
	}), opToken))
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	var roleID string
	for _, r := range roles.Msg.Roles {
		if r.Name == "e2e-authz.reader" {
			roleID = r.Id
		}
	}
	if roleID == "" {
		t.Fatal("manifest role e2e-authz.reader not found after apply")
	}

	tokens := login(t)
	sc := anubisv1connect.NewSessionServiceClient(http.DefaultClient, baseURL)
	me, err := sc.GetMe(ctx, bearer(connect.NewRequest(&anubisv1.GetMeRequest{}), tokens.AccessToken))
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if _, err := aza.CreateGrant(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateGrantRequest{
		IdentityId: me.Msg.IdentityId, RoleId: roleID, Reason: "e2e load probe",
	}), opToken)); err != nil && connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("create grant: %v", err)
	}
	return me.Msg.IdentityId, "e2e-authz:probe:read"
}
