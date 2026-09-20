package feed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

// A feed has no business at a metadata endpoint — egress.AllowHost says so,
// and it is checked before the request goes out.
//
// It was checked ONCE, against the URL the operator typed, while redirects
// were only counted. So a source pointed at a host the attacker controls
// could answer 302 and send the server wherever it liked: link-local,
// loopback, anything inside the network the policy exists to keep it out of.
// The operator needs anubis:sync:admin, which in a multi-tenant install is a
// CUSTOMER's admin — somebody who should not be able to make the host fetch
// its own cloud credentials.
func TestFeedRefusesARedirectIntoDeniedSpace(t *testing.T) {
	// The probe server is on loopback, so the FIRST hop has to be permitted
	// for the test to reach the redirect at all.
	t.Setenv("ANUBIS_SYNC_ALLOW_LOOPBACK", "1")

	for _, target := range []string{
		"http://169.254.169.254/latest/meta-data/", // cloud metadata
		"http://[fd00:ec2::254]/latest/meta-data/", // the IPv6 spelling
	} {
		t.Run(target, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target, http.StatusFound)
			}))
			defer srv.Close()

			cfg, _ := json.Marshal(map[string]string{"url": srv.URL})
			_, err := NewHTTPFetcher().Fetch(context.Background(),
				scopedomain.SyncSourceRecord{Kind: "http", Config: cfg})
			if err == nil {
				t.Fatal("the feed followed a redirect into denied space")
			}
			// It must fail because the POLICY refused it, not because the
			// address happened to be unroutable from this machine.
			if !strings.Contains(err.Error(), "metadata") &&
				!strings.Contains(err.Error(), "loopback") &&
				!strings.Contains(err.Error(), "not permitted") {
				t.Fatalf("refused for the wrong reason: %v", err)
			}
		})
	}
}

// The policy still has to let an ordinary feed through, or it closes the
// hole by removing the feature.
func TestFeedStillFollowsAnOrdinaryRedirect(t *testing.T) {
	t.Setenv("ANUBIS_SYNC_ALLOW_LOOPBACK", "1")

	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"ref":"a","name":"A","node_type":"unit"}]`))
	}))
	defer final.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirector.Close()

	cfg, _ := json.Marshal(map[string]string{"url": redirector.URL})
	rows, err := NewHTTPFetcher().Fetch(context.Background(),
		scopedomain.SyncSourceRecord{Kind: "http", Config: cfg})
	if err != nil {
		t.Fatalf("an ordinary redirect was refused: %v", err)
	}
	if len(rows) != 1 || rows[0].Ref != "a" {
		t.Fatalf("rows came back %+v", rows)
	}
}
