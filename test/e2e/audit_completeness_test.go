//go:build integration

package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
)

// An audit entry that is never written is worse than one that is wrong: the
// log looks complete, and nothing in the running system says otherwise.
//
// This is the test for that class of failure rather than for one instance of
// it. Creating an axis and a node type used to emit nothing at all — the code
// was passed where `audit_log.target_id` wants a uuid, the insert failed, and
// the auditor logged it to a stream nobody reads. Both facts are checked
// here: the entries have to appear, and the counter of dropped events must
// not move while they do.
func TestAdminActionsLeaveAnAuditTrail(t *testing.T) {
	requireServer(t)
	token := platformLogin(t)

	before := droppedAudits(t)

	axis := fmt.Sprintf("e2e_audit_%d", time.Now().UnixNano()%1e9)
	ensureAxis(t, token, axis, axis+"_root", axis+"_node")

	// Both of these emit through the path that used to throw the entry away.
	for _, action := range []string{"scope.axis_create", "scope.node_type_create"} {
		if !auditEntryAppears(t, token, action, axis) {
			t.Fatalf("%s left no audit entry mentioning %q", action, axis)
		}
	}

	if after := droppedAudits(t); after != before {
		t.Fatalf("the server dropped %d audit events during this test "+
			"(anubis_audit_dropped_total went %d -> %d)", after-before, before, after)
	}
}

// auditEntryAppears polls, because the auditor writes off the request path:
// the RPC returns before the entry lands, and a single immediate read would
// be testing the queue depth rather than the audit trail.
func auditEntryAppears(t *testing.T, token, action, mustMention string) bool {
	t.Helper()
	admin := anubisv1connect.NewTenantAdminServiceClient(http.DefaultClient, baseURL)
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := admin.QueryAudit(context.Background(),
			operatorBearer(connect.NewRequest(&anubisv1.QueryAuditRequest{
				Action: action, PageSize: 50,
			}), token))
		if err != nil {
			t.Fatalf("query audit: %v", err)
		}
		for _, e := range resp.Msg.Entries {
			if strings.Contains(e.DetailJson, mustMention) {
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// droppedAudits reads anubis_audit_dropped_total across every action label.
// Absent means zero: a counter with no observations is not exported.
func droppedAudits(t *testing.T) int {
	t.Helper()
	resp, err := http.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("read metrics: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	total := 0
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "anubis_audit_dropped_total{") {
			continue
		}
		fields := strings.Fields(line)
		n, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil {
			t.Fatalf("unparsable metric line %q", line)
		}
		total += n
	}
	return total
}

// A failed administrator login is the single entry a break-in investigation
// starts from, and for as long as the platform plane had no tenant to file
// under, none was ever written: the auditor discarded tenant-less events
// before it even counted them. So did every platform API key creation, every
// platform logout, and token.reuse_detected — the event alerting.md calls the
// highest-signal in the system, which could not fire for an operator at all.
//
// The installation now has a tenant id of its own, and this reads the trail
// back through the ordinary audit API to prove it is stored AND reachable.
func TestFailedPlatformLoginIsRecorded(t *testing.T) {
	requireServer(t)
	token := platformLogin(t)
	before := droppedAudits(t)

	who := fmt.Sprintf("ghost-%d", time.Now().UnixNano())
	pc := anubisv1connect.NewPlatformAuthServiceClient(http.DefaultClient, baseURL)
	_, err := pc.PlatformLogin(context.Background(),
		connect.NewRequest(&anubisv1.PlatformLoginRequest{
			Username: who, Password: "definitely-not-the-password",
		}))
	if err == nil {
		t.Fatal("a login with a made-up username succeeded")
	}

	// No tenant header: an operator asking without one is asking about the
	// installation, which is where platform-plane events live.
	admin := anubisv1connect.NewTenantAdminServiceClient(http.DefaultClient, baseURL)
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, qerr := admin.QueryAudit(context.Background(),
			bearer(connect.NewRequest(&anubisv1.QueryAuditRequest{
				Action: "platform.login", PageSize: 50,
			}), token))
		if qerr != nil {
			t.Fatalf("query installation audit: %v", qerr)
		}
		for _, e := range resp.Msg.Entries {
			if strings.Contains(e.DetailJson, who) {
				if e.Result != "deny" {
					t.Fatalf("failed login recorded as %q", e.Result)
				}
				if after := droppedAudits(t); after != before {
					t.Fatalf("audit events were dropped: %d -> %d", before, after)
				}
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no audit entry for the failed login of %q", who)
		}
		time.Sleep(250 * time.Millisecond)
	}
}
