package xlsx

import "strings"

// The types below map the SpreadsheetML parts this package reads. They are
// wire-format shapes, not domain types, which is why they live together in
// one file instead of one per struct — they only ever exist between
// encoding/xml and the Sheet values Read returns.

// xlWorkbook is xl/workbook.xml: the sheet order and the relationship id
// that points at each sheet's own part.
type xlWorkbook struct {
	Sheets []struct {
		Name string `xml:"name,attr"`
		RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
	} `xml:"sheets>sheet"`
}

// xlRels is any .rels part: relationship id to part target.
type xlRels struct {
	Rels []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

// xlSST is xl/sharedStrings.xml, the string table most writers use instead
// of repeating text inline.
type xlSST struct {
	Items []xlSI `xml:"si"`
}

// xlSI is one shared or inline string. Excel splits a string that carries
// mixed formatting into runs, so the value is the concatenation of every
// <t> beneath it rather than the first one.
type xlSI struct {
	T    string `xml:"t"`
	Runs []struct {
		T string `xml:"t"`
	} `xml:"r"`
}

func (s xlSI) value() string {
	if len(s.Runs) == 0 {
		return s.T
	}
	var b strings.Builder
	b.WriteString(s.T)
	for _, r := range s.Runs {
		b.WriteString(r.T)
	}
	return b.String()
}

// xlStyles is the slice of xl/styles.xml that decides whether a numeric
// cell is really a date: cellXfs maps a cell's s attribute to a number
// format id, and numFmts carries the non-builtin format codes.
type xlStyles struct {
	NumFmts []struct {
		ID   int    `xml:"numFmtId,attr"`
		Code string `xml:"formatCode,attr"`
	} `xml:"numFmts>numFmt"`
	Xfs []struct {
		NumFmtID int `xml:"numFmtId,attr"`
	} `xml:"cellXfs>xf"`
}

// xlRow is one <row> of a worksheet. Rows and cells are both sparse: r
// carries the true position of each.
type xlRow struct {
	R     int      `xml:"r,attr"`
	Cells []xlCell `xml:"c"`
}

// xlCell is one cell. T is the value type ("s" shared, "inlineStr", "str"
// formula result, "b" boolean, "e" error, empty for numeric), S indexes
// cellXfs, V holds the raw value and IS an inline string.
type xlCell struct {
	R  string `xml:"r,attr"`
	T  string `xml:"t,attr"`
	S  int    `xml:"s,attr"`
	V  string `xml:"v"`
	IS *xlSI  `xml:"is"`
}
