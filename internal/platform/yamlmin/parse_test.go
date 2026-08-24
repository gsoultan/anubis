package yamlmin

import (
	"strings"
	"testing"
)

const sample = `
# Anubis configuration
database:
  host: localhost
  port: 7449
  name: anubis
  user: anubis
  password: "enc:v1:AbC/dEf+g=="
  ssl: false
server:
  listen: ":7448"
  issuer: https://id.example.com   # trailing comment
  origins:
    - https://console.example.com
    - https://admin.example.com
`

func TestParseSample(t *testing.T) {
	doc, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Str("database", "host"); got != "localhost" {
		t.Errorf("host = %q", got)
	}
	if got := doc.Int(0, "database", "port"); got != 7449 {
		t.Errorf("port = %d", got)
	}
	if got := doc.Bool(true, "database", "ssl"); got != false {
		t.Errorf("ssl = %v", got)
	}
	if got := doc.Str("server", "listen"); got != ":7448" {
		t.Errorf("listen = %q", got)
	}
	// A trailing comment is not part of the value.
	if got := doc.Str("server", "issuer"); got != "https://id.example.com" {
		t.Errorf("issuer = %q", got)
	}
	if got := doc.Strings("server", "origins"); len(got) != 2 || got[0] != "https://console.example.com" {
		t.Errorf("origins = %v", got)
	}
	if doc.Keys[0] != "database" || doc.Keys[1] != "server" {
		t.Errorf("key order not preserved: %v", doc.Keys)
	}
}

// A base64 password can contain '/', '+', '=' and '#'. Truncating at a '#'
// that is not a comment would corrupt the secret and produce a connection
// failure nobody could explain from the config file.
func TestUnquotedValueKeepsHashWithoutSpace(t *testing.T) {
	doc, err := Parse([]byte("database:\n  password: abc#def\n  other: keep # dropped\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Str("database", "password"); got != "abc#def" {
		t.Errorf("password = %q, want abc#def", got)
	}
	if got := doc.Str("database", "other"); got != "keep" {
		t.Errorf("other = %q, want keep", got)
	}
}

func TestQuotedStrings(t *testing.T) {
	doc, err := Parse([]byte(`a: "with: colon"` + "\n" +
		`b: "tab\there"` + "\n" +
		`c: 'it''s literal'` + "\n" +
		`d: "trailing space "` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"a": "with: colon", "b": "tab\there", "c": "it's literal", "d": "trailing space ",
	} {
		if got := doc.Str(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestEmptyDocumentIsEmptyMapping(t *testing.T) {
	for _, src := range []string{"", "\n\n", "# only a comment\n"} {
		doc, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if doc.Kind != KindMap || len(doc.Keys) != 0 {
			t.Errorf("%q did not parse as an empty mapping", src)
		}
	}
}

func TestMissingPathsAreSafe(t *testing.T) {
	doc, _ := Parse([]byte("a:\n  b: 1\n"))
	if doc.Str("nope", "deeper") != "" {
		t.Error("a missing path should read empty")
	}
	if doc.Int(42, "a", "missing") != 42 {
		t.Error("a missing int should fall back to the default")
	}
	if doc.Bool(true, "x") != true {
		t.Error("a missing bool should fall back to the default")
	}
	if doc.Has("a", "c") {
		t.Error("Has reported a key that is not there")
	}
}

// Everything this parser does not implement must fail loudly. Silently
// misreading a construct boots an auth server on settings nobody wrote.
func TestUnsupportedConstructsAreRejected(t *testing.T) {
	cases := map[string]string{
		"anchor":         "a: &anchor v\n",
		"alias":          "a: *anchor\n",
		"tag":            "a: !!str v\n",
		"flow map":       "a: {b: 1}\n",
		"flow seq":       "a: [1, 2]\n",
		"block scalar":   "a: |\n  text\n",
		"folded":         "a: >\n  text\n",
		"multi doc":      "---\na: 1\n",
		"tab indent":     "a:\n\tb: 1\n",
		"no colon":       "just a line\n",
		"empty key":      ": value\n",
		"quoted key":     "\"a\": 1\n",
		"duplicate":      "a: 1\na: 2\n",
		"bad indent":     "a: 1\n   b: 2\n",
		"leading indent": "  a: 1\n",
		"unterminated":   "a: \"open\n",
		"bad escape":     "a: \"\\q\"\n",
		"top seq":        "- one\n",
	}
	for name, src := range cases {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

// Every rejection has to name the line, or the operator is hunting.
func TestErrorsNameTheLine(t *testing.T) {
	_, err := Parse([]byte("a: 1\nb: 2\nc: {flow: 1}\n"))
	if err == nil || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("error should name line 3, got %v", err)
	}
}

func TestDeepNestingIsBounded(t *testing.T) {
	var b strings.Builder
	for i := 0; i < maxDepth+8; i++ {
		b.WriteString(strings.Repeat(" ", i))
		b.WriteString("k:\n")
	}
	if _, err := Parse([]byte(b.String())); err == nil {
		t.Fatal("unbounded nesting should be rejected")
	}
}

func TestScalarListShorthand(t *testing.T) {
	doc, _ := Parse([]byte("hosts: one\n"))
	if got := doc.Strings("hosts"); len(got) != 1 || got[0] != "one" {
		t.Errorf("a lone scalar should read as a one-element list, got %v", got)
	}
}
