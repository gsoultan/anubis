//go:build integration

package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"testing"

	"connectrpc.com/connect"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
)

// Scope structures usually live somewhere else — an ERP, a CRM, another
// database entirely. These tests drive the three source kinds of
// migrations/0017 end to end: the server opens the SOURCE's own connection,
// pulls the feed, sorts it parents-first, and reconciles by external_ref.

func scopeClient() anubisv1connect.ScopeAdminServiceClient {
	return anubisv1connect.NewScopeAdminServiceClient(http.DefaultClient, baseURL)
}

// ensureAxis creates an axis and its two node types, tolerating re-runs.
func ensureAxis(t *testing.T, token, axis, rootType, childType string) {
	t.Helper()
	ctx := context.Background()
	sc := scopeClient()
	_, _ = sc.CreateScopeAxis(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateScopeAxisRequest{
		Axis: &anubisv1.ScopeAxis{Code: axis, DisplayName: axis, DefaultEffect: "unconstrained", SortOrder: 90},
	}), token))
	_, _ = sc.CreateScopeNodeType(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateScopeNodeTypeRequest{
		Type: &anubisv1.ScopeNodeType{Code: rootType, Axis: axis, DisplayName: rootType},
	}), token))
	_, _ = sc.CreateScopeNodeType(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateScopeNodeTypeRequest{
		Type: &anubisv1.ScopeNodeType{Code: childType, Axis: axis, DisplayName: childType,
			ParentTypes: []string{rootType, childType}},
	}), token))
}

// createSource registers the source, or REUSES the existing one for this
// axis — the schema allows exactly one source of truth per structure, and a
// test that skipped on re-run would prove nothing.
func createSource(t *testing.T, token, axis, kind, config string) string {
	t.Helper()
	ctx := context.Background()
	resp, err := scopeClient().CreateSyncSource(ctx,
		operatorBearer(connect.NewRequest(&anubisv1.CreateSyncSourceRequest{
			Source: &anubisv1.SyncSource{Axis: axis, Kind: kind, ConfigJson: config},
		}), token))
	if err == nil {
		return resp.Msg.Source.Id
	}
	if connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("create sync source: %v", err)
	}
	list, lerr := scopeClient().ListSyncSources(ctx,
		operatorBearer(connect.NewRequest(&anubisv1.ListSyncSourcesRequest{}), token))
	if lerr != nil {
		t.Fatalf("list sync sources: %v", lerr)
	}
	for _, s := range list.Msg.Sources {
		if s.Axis == axis {
			// Point the existing source at THIS run's config (the previous
			// run's httptest port is long gone) — the same call an operator
			// makes to rotate a DSN password or move a feed.
			if _, uerr := scopeClient().UpdateSyncSource(ctx,
				operatorBearer(connect.NewRequest(&anubisv1.UpdateSyncSourceRequest{
					Source: &anubisv1.SyncSource{Id: s.Id, Status: "active", ConfigJson: config},
				}), token)); uerr != nil {
				t.Fatalf("update sync source: %v", uerr)
			}
			return s.Id
		}
	}
	t.Fatalf("source for axis %q exists but was not listed", axis)
	return ""
}

func runSync(t *testing.T, token, sourceID string, dry bool) map[string]any {
	t.Helper()
	resp, err := scopeClient().RunSync(context.Background(),
		operatorBearer(connect.NewRequest(&anubisv1.RunSyncRequest{SourceId: sourceID, Dry: dry}), token))
	if err != nil {
		t.Fatalf("run sync: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(resp.Msg.ReportJson), &report); err != nil {
		t.Fatalf("report: %v (%s)", err, resp.Msg.ReportJson)
	}
	if errs, ok := report["errors"].([]any); ok && len(errs) > 0 {
		t.Fatalf("sync reported row errors: %v", errs)
	}
	return report
}

func num(t *testing.T, report map[string]any, key string) int {
	t.Helper()
	v, ok := report[key].(float64)
	if !ok {
		t.Fatalf("report missing %q: %v", key, report)
	}
	return int(v)
}

// TestScopeSyncFromHTTPFeed: the CRM-style case. The feed deliberately lists
// a CHILD BEFORE ITS PARENT — no SQL ORDER BY and few JSON APIs guarantee
// otherwise, so the server must sort before reconciling.
func TestScopeSyncFromHTTPFeed(t *testing.T) {
	requireServer(t)
	token := platformLogin(t)
	const axis = "e2e_http_axis"
	ensureAxis(t, token, axis, "e2e_http_root", "e2e_http_node")

	rows := []map[string]string{
		{"ref": "H-CHILD", "parent_ref": "H-MID", "name": "Child"}, // before its parent
		{"ref": "H-ROOT", "name": "Root"},
		{"ref": "H-MID", "parent_ref": "H-ROOT", "name": "Middle"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer feed-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(rows)
	}))
	defer srv.Close()

	src := createSource(t, token, axis, "http", fmt.Sprintf(
		`{"url":%q,"auth_header":"Bearer feed-token","default_node_type":"e2e_http_node"}`, srv.URL))

	// A dry run on a FRESH axis must resolve parents created earlier in the
	// same run (migration 0021) — otherwise it cries wolf on a clean feed.
	dry := runSync(t, token, src, true)
	if dry["dry"] != true {
		t.Fatal("dry run must report dry=true")
	}
	if got := num(t, dry, "added") + num(t, dry, "unchanged"); got != 3 {
		t.Fatalf("dry run: want 3 rows accounted for, got %d (%v)", got, dry)
	}

	applied := runSync(t, token, src, false)
	if got := num(t, applied, "added") + num(t, applied, "unchanged"); got != 3 {
		t.Fatalf("apply: want 3 rows accounted for, got %d (%v)", got, applied)
	}
	// Idempotence: the same feed twice changes nothing and archives nothing.
	again := runSync(t, token, src, false)
	if num(t, again, "unchanged") != 3 || num(t, again, "archived") != 0 {
		t.Fatalf("second run not idempotent: %v", again)
	}
}

// TestScopeSyncFromExternalDatabase proves the db_query path: a SEPARATE
// connection opened from the source's own dsn. The DSN may point anywhere;
// here it points at the dev database itself, which is enough to exercise
// connect + column mapping + reconcile without a second container.
func TestScopeSyncFromExternalDatabase(t *testing.T) {
	requireServer(t)
	dsn := os.Getenv("ANUBIS_DB_URL")
	if dsn == "" {
		t.Skip("ANUBIS_DB_URL not set")
	}
	token := platformLogin(t)
	const axis = "e2e_db_axis"
	ensureAxis(t, token, axis, "e2e_db_root", "e2e_db_node")

	// A literal feed, shaped exactly like a real ERP query would be, and
	// again ordered child-first to prove the sort.
	query := `SELECT * FROM (VALUES
	    ('D-LEAF','D-BRANCH','Leaf'),
	    ('D-ROOT',NULL,'Root'),
	    ('D-BRANCH','D-ROOT','Branch')
	  ) AS t(ref, parent_ref, name)`
	cfg, _ := json.Marshal(map[string]string{
		"dsn": dsn, "query": query, "default_node_type": "e2e_db_node",
	})
	src := createSource(t, token, axis, "db_query", string(cfg))

	applied := runSync(t, token, src, false)
	if got := num(t, applied, "added") + num(t, applied, "unchanged"); got != 3 {
		t.Fatalf("external db sync: want 3 rows accounted for, got %d (%v)", got, applied)
	}

	// The hierarchy must survive the trip: leaf under branch under root.
	nodes, err := scopeClient().ListScopeNodes(context.Background(),
		operatorBearer(connect.NewRequest(&anubisv1.ListScopeNodesRequest{Axis: axis}), token))
	if err != nil {
		t.Fatal(err)
	}
	byRef := map[string]*anubisv1.ScopeNode{}
	byID := map[string]*anubisv1.ScopeNode{}
	for _, n := range nodes.Msg.Nodes {
		byID[n.Id] = n
		if n.ExternalRef != "" {
			byRef[n.ExternalRef] = n
		}
	}
	leaf, branch := byRef["D-LEAF"], byRef["D-BRANCH"]
	if leaf == nil || branch == nil {
		t.Fatalf("synced nodes missing: %v", byRef)
	}
	if leaf.ParentId != branch.Id {
		t.Fatalf("hierarchy lost: D-LEAF parent is %q, want D-BRANCH %q",
			byID[leaf.ParentId].GetExternalRef(), branch.Id)
	}
}

// A source that cannot be reached must fail LOUDLY rather than report an
// empty feed — an empty feed would archive every node it was meant to
// confirm.
func TestScopeSyncUnreachableSourceDoesNotArchive(t *testing.T) {
	requireServer(t)
	token := platformLogin(t)
	const axis = "e2e_dead_axis"
	ensureAxis(t, token, axis, "e2e_dead_root", "e2e_dead_node")

	src := createSource(t, token, axis, "http",
		`{"url":"http://127.0.0.1:1/gone","default_node_type":"e2e_dead_node"}`)

	_, err := scopeClient().RunSync(context.Background(),
		operatorBearer(connect.NewRequest(&anubisv1.RunSyncRequest{SourceId: src}), token))
	if err == nil {
		t.Fatal("unreachable feed reported success")
	}
	if code := connect.CodeOf(err); code != connect.CodeUnavailable {
		t.Fatalf("want unavailable, got %v (%v)", code, err)
	}
}

// Config that cannot work must be rejected at creation, not at 3am.
func TestScopeSyncRejectsBadConfig(t *testing.T) {
	requireServer(t)
	token := platformLogin(t)
	ctx := context.Background()
	cases := []struct{ kind, config string }{
		{"http", `{}`},
		{"http", `{"url":"file:///etc/passwd"}`},
		{"db_query", `{"dsn":"postgres://x/y"}`},
		{"db_table", `{"dsn":"postgres://x/y","table":"t"}`},
		{"carrier_pigeon", `{}`},
	}
	for _, c := range cases {
		_, err := scopeClient().CreateSyncSource(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateSyncSourceRequest{
			Source: &anubisv1.SyncSource{Axis: "e2e_reject_axis", Kind: c.kind, ConfigJson: c.config},
		}), token))
		if err == nil {
			t.Errorf("accepted invalid %s config: %s", c.kind, c.config)
			continue
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s %s: want invalid_argument, got %v", c.kind, c.config, connect.CodeOf(err))
		}
	}
}

// History is what the reconciler already recorded. The table has existed
// since migration 0017 and nothing ever read it, so "when did this last
// sync and what did it change" had no answer at 3am.
func TestScopeSyncRecordsHistory(t *testing.T) {
	requireServer(t)
	token := platformLogin(t)
	const axis = "e2e_hist_axis"
	ensureAxis(t, token, axis, "e2e_hist_root", "e2e_hist_node")

	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"ref":"H-1","name":"History One"}]`))
	}))
	defer feed.Close()
	src := createSource(t, token, axis, "http",
		fmt.Sprintf(`{"url":%q,"default_node_type":"e2e_hist_node"}`, feed.URL))

	before := len(listRuns(t, token, src))
	runSync(t, token, src, true) // a dry run is history too
	runSync(t, token, src, false)

	runs := listRuns(t, token, src)
	if len(runs) < before+2 {
		t.Fatalf("history did not record both runs: had %d, now %d", before, len(runs))
	}
	// Newest first, and the dry run must be distinguishable from the real
	// one — they leave identical counts behind.
	if runs[0].Dry == runs[1].Dry {
		t.Fatalf("dry and applied runs are indistinguishable: %+v", runs[:2])
	}
	if runs[0].StartedAt == 0 || runs[0].ReportJson == "" {
		t.Fatalf("run recorded without a time or a report: %+v", runs[0])
	}
}

func listRuns(t *testing.T, token, sourceID string) []*anubisv1.SyncRun {
	t.Helper()
	resp, err := scopeClient().ListSyncRuns(context.Background(),
		operatorBearer(connect.NewRequest(&anubisv1.ListSyncRunsRequest{SourceId: sourceID}), token))
	if err != nil {
		t.Fatalf("list sync runs: %v", err)
	}
	return resp.Msg.Runs
}

// The structure a company keeps is rarely in Postgres, and this proves the
// whole path against one that is not: create a db_table source pointed at a
// real MySQL server, run the sync through the API, and read the nodes back.
//
// The fetcher's own MySQL test covers the driver; this covers everything
// after it — the RPC, the reconciler, parents-first ordering across an engine
// with no NULLS FIRST, and the rows landing in the right axis.
//
// Skipped unless ANUBIS_TEST_MYSQL_DSN names a server, so the ordinary suite
// needs no second database.
func TestScopeSyncFromMySQL(t *testing.T) {
	requireServer(t)
	dsn := os.Getenv("ANUBIS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("ANUBIS_TEST_MYSQL_DSN not set")
	}
	table := seedMySQL(t, dsn)
	token := platformLogin(t)
	ctx := context.Background()
	const axis = "e2e_mysql_axis"
	ensureAxis(t, token, axis, "e2e_mysql_root", "e2e_mysql_node")

	src := createSource(t, token, axis, "db_table", fmt.Sprintf(
		`{"dsn":%q,"table":%q,"default_node_type":"e2e_mysql_node",
		  "columns":{"ref":"code","parent_ref":"parent","name":"label"}}`, dsn, table))

	report := runSync(t, token, src, false)
	if errs, _ := report["errors"].([]any); len(errs) > 0 {
		t.Fatalf("sync reported errors: %v", errs)
	}
	// On a fresh database these are adds; on a re-run they are unchanged.
	// Either way the sync must have seen every row.
	if seen := num(t, report, "added") + num(t, report, "unchanged"); seen < 3 {
		t.Fatalf("sync saw %d of 3 rows: %v", seen, report)
	}

	list, err := scopeClient().ListScopeNodes(ctx,
		operatorBearer(connect.NewRequest(&anubisv1.ListScopeNodesRequest{Axis: axis}), token))
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	byRef := map[string]*anubisv1.ScopeNode{}
	byID := map[string]*anubisv1.ScopeNode{}
	for _, n := range list.Msg.Nodes {
		byID[n.Id] = n
		if n.ExternalRef != "" {
			byRef[n.ExternalRef] = n
		}
	}
	div, team := byRef["DIV"], byRef["TEAM"]
	if div == nil || team == nil {
		t.Fatalf("MySQL rows did not become nodes: %v", byRef)
	}
	if team.ParentId != div.Id {
		t.Fatalf("hierarchy lost across MySQL: TEAM's parent is %q, want DIV",
			byID[team.ParentId].GetExternalRef())
	}
	if div.Name != "Engineering Division" {
		t.Fatalf("column mapping wrong: %q", div.Name)
	}
	// AAA sorts before its parent TEAM, so a fetcher that returned rows in
	// natural order would try to attach a child to a node that does not exist
	// yet. Reaching this line means the ordering survived an engine with no
	// NULLS FIRST of its own.
	alpha := byRef["AAA"]
	if alpha == nil {
		t.Fatal("grandchild AAA never landed")
	}
	if alpha.ParentId != team.Id {
		t.Fatalf("AAA attached to %q, want TEAM",
			byID[alpha.ParentId].GetExternalRef())
	}
}

// seedMySQL builds the little org chart the test reads back, so the test needs
// no set-up beyond a reachable server. It returns the table name.
//
// The mysql:// URL is translated here the same way the fetcher translates it;
// duplicating three lines keeps the seeding honest about what it connects to
// without exporting the dialect internals.
func seedMySQL(t *testing.T, dsn string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("ANUBIS_TEST_MYSQL_DSN is not a URL: %v", err)
	}
	driverDSN := u.User.String() + "@tcp(" + u.Host + ")" + u.Path
	db, err := sql.Open("mysql", driverDSN)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	defer db.Close()

	const table = "anubis_e2e_org"
	// AAA sorts before the parent it names, so natural row order is wrong on
	// purpose: the sync has to order roots first and resolve forward
	// references, on an engine that has no NULLS FIRST.
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS ` + table + ` (
		   code VARCHAR(32) PRIMARY KEY, parent VARCHAR(32) NULL, label VARCHAR(128))`,
		`INSERT IGNORE INTO ` + table + ` VALUES
		   ('AAA','TEAM','Alpha Squad'),
		   ('TEAM','DIV','Platform Team'),
		   ('DIV',NULL,'Engineering Division')`,
	} {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("seed mysql: %v", err)
		}
	}
	return table
}
