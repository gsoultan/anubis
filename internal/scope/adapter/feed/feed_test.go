package feed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

func source(kind, cfg string) scopedomain.SyncSourceRecord {
	return scopedomain.SyncSourceRecord{Kind: kind, Axis: "cost_center", Config: []byte(cfg)}
}

func TestHTTPFetch(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode([]scopedomain.SyncFeedRow{
			{Ref: "CC-1", Name: "Division One"},
			{Ref: "CC-1-A", ParentRef: "CC-1", Name: "Team A"},
		})
	}))
	defer srv.Close()

	rows, err := NewHTTPFetcher().Fetch(context.Background(),
		source("http", `{"url":"`+srv.URL+`","auth_header":"Bearer t0ken"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1].ParentRef != "CC-1" {
		t.Fatalf("rows: %+v", rows)
	}
	if gotAuth != "Bearer t0ken" {
		t.Errorf("auth header not forwarded: %q", gotAuth)
	}
}

func TestHTTPFetchRejects(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	if _, err := NewHTTPFetcher().Fetch(context.Background(),
		source("http", `{"url":"`+bad.URL+`"}`)); err == nil {
		t.Fatal("500 accepted")
	}

	// Non-http schemes must not become an SSRF primitive into file:// etc.
	if _, err := NewHTTPFetcher().Fetch(context.Background(),
		source("http", `{"url":"file:///etc/passwd"}`)); err == nil {
		t.Fatal("file:// scheme accepted")
	}
	if _, err := NewHTTPFetcher().Fetch(context.Background(),
		source("http", `{}`)); err == nil {
		t.Fatal("missing url accepted")
	}
}

// The db_table config names a table and columns. Those are admin-supplied,
// so a poisoned config must not smuggle SQL into the FOREIGN database.
func TestDBTableRejectsInjection(t *testing.T) {
	cases := []string{
		`{"dsn":"postgres://x/y","table":"customers; DROP TABLE users","columns":{"ref":"id","name":"n"}}`,
		`{"dsn":"postgres://x/y","table":"public.customers","columns":{"ref":"id\"; DROP TABLE users --","name":"n"}}`,
		`{"dsn":"postgres://x/y","table":"a.b.c","columns":{"ref":"id","name":"n"}}`,
		`{"dsn":"postgres://x/y","table":"customers","columns":{"name":"n"}}`,
		`{"dsn":"postgres://x/y","columns":{"ref":"id","name":"n"}}`,
	}
	for _, cfg := range cases {
		_, err := NewDBTableFetcher().Fetch(context.Background(), source("db_table", cfg))
		if err == nil {
			t.Errorf("accepted hostile config: %s", cfg)
			continue
		}
		if code := apperr.AsError(err).Code; code != apperr.ErrInvalidArgument.Code {
			t.Errorf("config %s: want invalid_argument, got %s", cfg, code)
		}
	}
}

func TestQuoteTable(t *testing.T) {
	got, err := quoteTable("public.customers")
	if err != nil || got != `"public"."customers"` {
		t.Fatalf("quoteTable = %q, %v", got, err)
	}
	if got, err := quoteTable("customers"); err != nil || got != `"customers"` {
		t.Fatalf("quoteTable = %q, %v", got, err)
	}
	for _, bad := range []string{"", "a b", "a;b", `a"b`, strings.Repeat("x", 64)} {
		if _, err := quoteTable(bad); err == nil {
			t.Errorf("quoteTable(%q) accepted", bad)
		}
	}
}

func TestDispatchUnknownKind(t *testing.T) {
	if _, err := NewFetcher().Fetch(context.Background(), source("carrier_pigeon", `{}`)); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestAsString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"x", "x"},
		{[]byte("y"), "y"},
		{int64(42), "42"},
	}
	for _, c := range cases {
		if got := asString(c.in); got != c.want {
			t.Errorf("asString(%v) = %q want %q", c.in, got, c.want)
		}
	}
}
