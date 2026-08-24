package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
)

func TestColumnName(t *testing.T) {
	cases := map[int]string{0: "A", 1: "B", 25: "Z", 26: "AA", 27: "AB", 51: "AZ", 52: "BA", 701: "ZZ", 702: "AAA"}
	for in, want := range cases {
		if got := ColumnName(in); got != want {
			t.Errorf("ColumnName(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestColumnIndexRoundTrip(t *testing.T) {
	for i := 0; i < 1000; i++ {
		if got := columnIndex(ColumnName(i) + "17"); got != i {
			t.Fatalf("columnIndex(%q) = %d, want %d", ColumnName(i)+"17", got, i)
		}
	}
}

func TestColumnIndexRejectsJunk(t *testing.T) {
	for _, ref := range []string{"", "12", "$A$1"} {
		if got := columnIndex(ref); got != -1 {
			t.Errorf("columnIndex(%q) = %d, want -1", ref, got)
		}
	}
}

// Excel refuses to open a package whose parts are not well-formed XML, and
// that failure surfaces to the operator as "unreadable content" with no
// clue which part is at fault. Parsing every part here turns that into a
// test failure instead.
func TestWritePartsAreWellFormedXML(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, []Sheet{{
		Name:    "People",
		Columns: []Column{{Header: "realm", Allowed: []string{"staff", "customer"}}, {Header: "username", Width: 30}},
		Rows:    [][]string{{"staff", "ada"}, {"", "grace"}},
	}}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"[Content_Types].xml": false, "_rels/.rels": false, "xl/workbook.xml": false,
		"xl/_rels/workbook.xml.rels": false, "xl/styles.xml": false,
		"xl/worksheets/sheet1.xml": false,
	}
	for _, f := range zr.File {
		if _, ok := want[f.Name]; ok {
			want[f.Name] = true
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(rc)
		for {
			if _, err := dec.Token(); err != nil {
				if err.Error() != "EOF" {
					t.Errorf("part %s is not well-formed: %v", f.Name, err)
				}
				break
			}
		}
		rc.Close()
	}
	for name, found := range want {
		if !found {
			t.Errorf("package is missing required part %s", name)
		}
	}
}

func TestWriteRejectsEmptyWorkbook(t *testing.T) {
	if err := Write(&bytes.Buffer{}, nil); err != ErrNoSheets {
		t.Fatalf("got %v, want ErrNoSheets", err)
	}
}

func TestSheetNamesAreExcelSafe(t *testing.T) {
	got := sheetNames([]Sheet{
		{Name: "People/Access"},
		{Name: ""},
		{Name: strings.Repeat("x", 40)},
		{Name: "People_Access"},
	})
	if got[0] != "People_Access" {
		t.Errorf("illegal rune not replaced: %q", got[0])
	}
	if got[1] != "Sheet2" {
		t.Errorf("empty name not defaulted: %q", got[1])
	}
	if len([]rune(got[2])) != 31 {
		t.Errorf("name not truncated to 31: %q (%d)", got[2], len([]rune(got[2])))
	}
	if got[3] == got[0] {
		t.Errorf("duplicate names not disambiguated: %q", got[3])
	}
}

func TestValidationsRenderedForAllowedValues(t *testing.T) {
	x := sheetXML(Sheet{
		Name: "S",
		Columns: []Column{
			{Header: "kind", Allowed: []string{"signin", "signout"}},
			{Header: "free"},
		},
	})
	if !strings.Contains(x, `<formula1>"signin,signout"</formula1>`) {
		t.Errorf("dropdown missing from sheet XML:\n%s", x)
	}
	if strings.Count(x, "<dataValidation ") != 1 {
		t.Errorf("want exactly one validation, got:\n%s", x)
	}
	// dataValidations must follow sheetData or Excel rejects the sheet.
	if strings.Index(x, "<dataValidations") < strings.Index(x, "</sheetData>") {
		t.Error("dataValidations must come after sheetData")
	}
}

// A comma cannot be represented in an inline list formula, so such a
// column has to stay free text rather than get a dropdown that splits the
// value in half.
func TestValidationSkippedWhenValueBreaksInlineList(t *testing.T) {
	if _, ok := inlineList([]string{"a,b", "c"}); ok {
		t.Error("a value containing a comma must not become a dropdown")
	}
	if _, ok := inlineList([]string{strings.Repeat("x", 300)}); ok {
		t.Error("an oversized list must not become a dropdown")
	}
	if got, ok := inlineList([]string{"a", "b"}); !ok || got != "a,b" {
		t.Errorf("inlineList = %q, %v", got, ok)
	}
}

func TestEscapeStripsForbiddenControlBytes(t *testing.T) {
	if got := escape("a\x00b"); strings.ContainsRune(got, 0) {
		t.Errorf("NUL survived escaping: %q", got)
	}
	if got := escape(`<&">`); got != `&lt;&amp;&#34;&gt;` {
		t.Errorf("escape = %q", got)
	}
}
