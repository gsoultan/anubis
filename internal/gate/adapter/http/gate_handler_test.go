package gatehttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gsoultan/anubis/internal/gate/snapshot"
)

// stubSnapshots is the manager's answer without the manager: whether there is
// data, and whether it is fresh enough to decide from.
type stubSnapshots struct {
	data  *snapshot.Data
	fresh bool
}

func (s stubSnapshots) Get(string) (*snapshot.Data, bool) { return s.data, s.fresh }

func gateRequest() *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/v1/gate/check", nil)
	r.Header.Set("X-Original-URI", "/reports/42")
	r.Header.Set("X-Original-Method", "GET")
	r.Header.Set("X-Anubis-Tenant", "acme")
	return r
}

// The gate serves entirely from the snapshot, so a stale one is the dangerous
// case: the data is right there and cheap to answer from, and answering from
// it means honouring access that may have been revoked minutes or hours ago.
// operations.md promises fail-closed past the ceiling. This is that promise.
func TestAStaleSnapshotIsRefusedNotServed(t *testing.T) {
	h := NewGateHandler("https://anubis.example", nil,
		stubSnapshots{data: &snapshot.Data{}, fresh: false})

	w := httptest.NewRecorder()
	h.Check(w, gateRequest())

	// The code alone proves nothing here: an empty snapshot denies everything
	// anyway, so a 403 could mean "policy said no" rather than "we refused to
	// look". The message is what distinguishes refusing-to-decide from
	// deciding — and it is the difference between fail-closed working and
	// merely appearing to.
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "snapshot unavailable") {
		t.Fatalf("a stale snapshot answered %d %q, want 403 \"authorization snapshot unavailable\" — "+
			"the gate decided from data past its max age instead of refusing",
			w.Code, strings.TrimSpace(w.Body.String()))
	}
}

// No snapshot at all is the same refusal, for the same reason: an instance
// that has never loaded cannot know what is allowed.
func TestNoSnapshotIsRefused(t *testing.T) {
	h := NewGateHandler("https://anubis.example", nil, stubSnapshots{})

	w := httptest.NewRecorder()
	h.Check(w, gateRequest())

	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "snapshot unavailable") {
		t.Fatalf("a missing snapshot answered %d %q, want 403 \"authorization snapshot unavailable\"",
			w.Code, strings.TrimSpace(w.Body.String()))
	}
}

// Fail-closed must not swallow a malformed request into the same answer: a
// caller sending no path has a bug, and telling them "forbidden" sends them
// looking for a permission problem that does not exist.
func TestAMalformedGateRequestIsNotADenial(t *testing.T) {
	h := NewGateHandler("https://anubis.example", nil,
		stubSnapshots{data: &snapshot.Data{}, fresh: true})

	r := httptest.NewRequest(http.MethodGet, "/v1/gate/check", nil)
	r.Header.Set("X-Anubis-Tenant", "acme") // no URI, no method
	w := httptest.NewRecorder()
	h.Check(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("a request with no path answered %d, want 400", w.Code)
	}
}
