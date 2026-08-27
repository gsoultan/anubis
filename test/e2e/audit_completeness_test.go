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
