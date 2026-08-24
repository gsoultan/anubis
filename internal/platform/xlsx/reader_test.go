package xlsx

import (
	"archive/zip"
	"bytes"
	"reflect"
	"testing"
)

func writeRead(t *testing.T, sheets []Sheet) []Sheet {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, sheets); err != nil {
		t.Fatal(err)
	}
	got, err := Read(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRoundTrip(t *testing.T) {
	in := []Sheet{
		{
			Name:    "People",
			Columns: []Column{{Header: "realm"}, {Header: "username"}, {Header: "email"}},
			Rows: [][]string{
				{"staff", "ada", "ada@example.com"},
				{"staff", "grace", "grace@example.com"},
			},
		},
		{
			Name:    "Grants",
			Columns: []Column{{Header: "username"}, {Header: "role"}},
			Rows:    [][]string{{"ada", "admin"}},
		},
	}
	got := writeRead(t, in)
	if len(got) != 2 {
		t.Fatalf("got %d sheets, want 2", len(got))
	}
	for i, want := range in {
		if got[i].Name != want.Name {
			t.Errorf("sheet %d name = %q, want %q", i, got[i].Name, want.Name)
		}
		if !reflect.DeepEqual(got[i].Header(), want.Header()) {
			t.Errorf("sheet %d header = %v, want %v", i, got[i].Header(), want.Header())
		}
		if !reflect.DeepEqual(got[i].Rows, want.Rows) {
			t.Errorf("sheet %d rows = %v, want %v", i, got[i].Rows, want.Rows)
		}
	}
}

// A gap in the middle of a row is stored by omitting the cell entirely. If
// the reader went positionally instead of reading each cell's reference,
// every value after the gap would shift one column left — silently
// importing an email address as a category.
func TestSparseCellsKeepTheirColumn(t *testing.T) {
	got := writeRead(t, []Sheet{{
		Name:    "S",
		Columns: []Column{{Header: "a"}, {Header: "b"}, {Header: "c"}},
		Rows:    [][]string{{"1", "", "3"}},
	}})
	want := []string{"1", "", "3"}
	if !reflect.DeepEqual(got[0].Rows[0], want) {
		t.Fatalf("row = %v, want %v", got[0].Rows[0], want)
	}
}

func TestValuesWithMarkupAndUnicodeSurvive(t *testing.T) {
	want := []string{"<b>&amp;</b>", "naïve “quoted”", "  padded  "}
	got := writeRead(t, []Sheet{{
		Name:    "S",
		Columns: []Column{{Header: "a"}, {Header: "b"}, {Header: "c"}},
		Rows:    [][]string{want},
	}})
	if !reflect.DeepEqual(got[0].Rows[0], want) {
		t.Fatalf("row = %#v, want %#v", got[0].Rows[0], want)
	}
}

func TestReadRejectsNonWorkbook(t *testing.T) {
	b := []byte("this is not a zip")
	if _, err := Read(bytes.NewReader(b), int64(len(b))); err == nil {
		t.Fatal("want an error for non-zip input")
	}
}

// build assembles a workbook part-by-part so the reader can be tested
// against the way Excel actually writes files — shared strings and styled
// numeric cells — rather than only against this package's own writer.
func build(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range parts {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func excelStyleWorkbook(t *testing.T, cells string) []byte {
	t.Helper()
	return build(t, map[string]string{
		"xl/workbook.xml": `<workbook xmlns="` + nsMain + `" xmlns:r="` + nsRel + `">` +
			`<sheets><sheet name="People" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="` + nsPkgRel + `">` +
			`<Relationship Id="rId1" Type="` + nsRel + `/worksheet" Target="worksheets/sheet1.xml"/>` +
			`</Relationships>`,
		"xl/sharedStrings.xml": `<sst xmlns="` + nsMain + `" count="3" uniqueCount="3">` +
			`<si><t>username</t></si>` +
			`<si><t>valid_until</t></si>` +
			`<si><r><t>a</t></r><r><t>da</t></r></si>` +
			`</sst>`,
		// Style 1 points at builtin format 14 (short date); style 2 at a
		// custom date code; style 3 is plain and must stay a number.
		"xl/styles.xml": `<styleSheet xmlns="` + nsMain + `">` +
			`<numFmts count="1"><numFmt numFmtId="165" formatCode="yyyy\-mm\-dd"/></numFmts>` +
			`<cellXfs count="4">` +
			`<xf numFmtId="0"/><xf numFmtId="14"/><xf numFmtId="165"/><xf numFmtId="3"/>` +
			`</cellXfs></styleSheet>`,
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="` + nsMain + `"><sheetData>` + cells +
			`</sheetData></worksheet>`,
	})
}

func TestReadResolvesSharedStringsIncludingRuns(t *testing.T) {
	b := excelStyleWorkbook(t, `<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>`+
		`<row r="2"><c r="A2" t="s"><v>2</v></c></row>`)
	got, err := Read(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if h := got[0].Header(); !reflect.DeepEqual(h, []string{"username", "valid_until"}) {
		t.Fatalf("header = %v", h)
	}
	// "ada" is stored as two formatting runs; both have to be joined.
	if got[0].Rows[0][0] != "ada" {
		t.Fatalf("run-split shared string = %q, want \"ada\"", got[0].Rows[0][0])
	}
}

// A date typed into a template is stored as a bare serial number. Reading
// it as a number would make every date column import as nonsense.
func TestReadConvertsDateSerials(t *testing.T) {
	b := excelStyleWorkbook(t,
		`<row r="1"><c r="A1" t="s"><v>0</v></c></row>`+
			`<row r="2"><c r="A2" s="1"><v>45658</v></c>`+ // builtin short date
			`<c r="B2" s="2"><v>46418</v></c>`+ // custom date code
			`<c r="C2" s="3"><v>46418</v></c>`+ // plain number, must not convert
			`<c r="D2" s="1"><v>45658.5</v></c></row>`) // date with a time part
	got, err := Read(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	row := got[0].Rows[0]
	want := []string{"2025-01-01", "2027-01-31", "46418", "2025-01-01T12:00:00Z"}
	if !reflect.DeepEqual(row, want) {
		t.Fatalf("row = %#v, want %#v", row, want)
	}
}

func TestIsDateFormat(t *testing.T) {
	cases := map[string]bool{
		`yyyy-mm-dd`:      true,
		`d/m/yy h:mm`:     true,
		`0.00`:            false,
		`#,##0`:           false,
		`"d"#,##0`:        false, // a literal d inside quotes is not a day
		`[Red]0.00`:       false,
		`\d0`:             false, // an escaped d is a literal
		`General`:         false,
		`"$"#,##0.00_);;`: false,
	}
	for code, want := range cases {
		if got := isDateFormat(code); got != want {
			t.Errorf("isDateFormat(%q) = %v, want %v", code, got, want)
		}
	}
}

// Excel omits empty rows, so row 4 can follow row 1. The gap has to be
// preserved or an import report points the operator at the wrong line.
func TestSparseRowsKeepTheirNumber(t *testing.T) {
	b := excelStyleWorkbook(t,
		`<row r="1"><c r="A1" t="inlineStr"><is><t>h</t></is></c></row>`+
			`<row r="4"><c r="A4" t="inlineStr"><is><t>v</t></is></c></row>`)
	got, err := Read(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got[0].Rows); n != 3 {
		t.Fatalf("got %d data rows, want 3 (two blanks then the value)", n)
	}
	if got[0].Rows[2][0] != "v" {
		t.Fatalf("value landed at the wrong row: %#v", got[0].Rows)
	}
}

func TestReadHandlesBooleanAndFormulaCells(t *testing.T) {
	b := excelStyleWorkbook(t,
		`<row r="1"><c r="A1" t="inlineStr"><is><t>h</t></is></c></row>`+
			`<row r="2"><c r="A2" t="b"><v>1</v></c><c r="B2" t="b"><v>0</v></c>`+
			`<c r="C2" t="str"><v>computed</v></c></row>`)
	got, err := Read(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"true", "false", "computed"}
	if !reflect.DeepEqual(got[0].Rows[0], want) {
		t.Fatalf("row = %#v, want %#v", got[0].Rows[0], want)
	}
}
