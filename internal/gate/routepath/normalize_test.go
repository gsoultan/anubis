package routepath

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// The shared adversarial corpus (testdata/normalize_corpus.txt) runs against
// EVERY path normaliser in the system — the gap between two normalisers is
// the bypass. Format: input<TAB>expected, where expected REJECT means the
// input must fail closed.
func TestNormalizeCorpus(t *testing.T) {
	f, err := os.Open("testdata/normalize_corpus.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		text := sc.Text()
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		parts := strings.Split(text, "\t")
		if len(parts) != 2 {
			t.Fatalf("corpus line %d malformed: %q", line, text)
		}
		got, err := NormalizePath(parts[0])
		if parts[1] == "REJECT" {
			if err == nil {
				t.Errorf("line %d: %q normalised to %q, want REJECT", line, parts[0], got)
			}
			continue
		}
		if err != nil {
			t.Errorf("line %d: %q rejected, want %q", line, parts[0], parts[1])
			continue
		}
		if got != parts[1] {
			t.Errorf("line %d: %q -> %q, want %q", line, parts[0], got, parts[1])
		}
	}
}

func FuzzNormalizePath(f *testing.F) {
	f.Add("/a/b/../c")
	f.Add("/%2e%2e/etc/passwd")
	f.Add("//a//b/")
	f.Add("/..0") // a segment merely CONTAINING dots is a name, not traversal
	f.Add("/0%23") // decoded '#' — delimiter to a re-parser, data to us
	f.Add("/a%3fb")
	f.Fuzz(func(t *testing.T, s string) {
		out, err := NormalizePath(s)
		if err != nil {
			return
		}
		// Invariants of every accepted path:
		if out == "" || out[0] != '/' {
			t.Fatalf("accepted %q -> %q without leading slash", s, out)
		}
		if strings.Contains(out, "//") || strings.Contains(out, "%") ||
			strings.Contains(out, ";") {
			t.Fatalf("accepted %q -> %q with ambiguity", s, out)
		}
		// No dot-SEGMENT survives normalisation. Segment-level on purpose:
		// "..0" and "a..b" are legitimate names, and rejecting them would
		// 403 real resources without closing any traversal.
		for _, seg := range strings.Split(strings.TrimPrefix(out, "/"), "/") {
			if seg == "." || seg == ".." {
				t.Fatalf("accepted %q -> %q with a dot-segment", s, out)
			}
		}
		// Idempotent: normalising a normalised path is identity.
		again, err := NormalizePath(out)
		if err != nil || again != out {
			t.Fatalf("not idempotent: %q -> %q -> %q (%v)", s, out, again, err)
		}
	})
}

func TestMatcher(t *testing.T) {
	cases := []struct {
		pattern, path string
		ok            bool
		param         string
	}{
		{"/invoices/{id}", "/invoices/42", true, "42"},
		{"/invoices/{id}", "/invoices", false, ""},
		{"/invoices/{id}", "/invoices/42/lines", false, ""},
		{"/static/**", "/static/css/app.css", true, ""},
		{"/static/**", "/static", false, ""},
		{"/health", "/health", true, ""},
		{"/health", "/healthz", false, ""},
		{"/a/*/c", "/a/b/c", true, ""},
		{"/a/*/c", "/a/b/d", false, ""},
	}
	for _, c := range cases {
		params, ok := pathMatch(c.pattern, c.path)
		if ok != c.ok {
			t.Errorf("pathMatch(%q,%q)=%v want %v", c.pattern, c.path, ok, c.ok)
			continue
		}
		if c.param != "" && params["id"] != c.param {
			t.Errorf("pathMatch(%q,%q) id=%q want %q", c.pattern, c.path, params["id"], c.param)
		}
	}
}
