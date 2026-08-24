// Package xlsx reads and writes Microsoft Excel (.xlsx) workbooks using
// nothing but the standard library, as ADR-0002 requires: a workbook is a
// zip of XML parts, so archive/zip and encoding/xml are the whole
// dependency list.
//
// The supported surface is deliberately the subset an import template
// needs — a header row, text cells, column widths, a frozen pane and
// dropdown validations. It is not a general spreadsheet engine: there are
// no formulas, charts or merged cells.
package xlsx

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	nsMain    = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	nsRel     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsPkgRel  = "http://schemas.openxmlformats.org/package/2006/relationships"
	nsContent = "http://schemas.openxmlformats.org/package/2006/content-types"

	xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`

	// maxSheets bounds both directions of the codec. Excel itself has no
	// such limit, but a workbook arriving over an RPC is attacker-supplied
	// input and every bound here is one fewer way to exhaust the server.
	maxSheets  = 64
	maxColumns = 16384

	defaultWidth = 22.0
)

// ErrNoSheets is returned when asked to write an empty workbook. Excel
// cannot open a package with zero worksheets, so this fails loudly here
// rather than producing a file that fails at the operator's desk.
var ErrNoSheets = errors.New("xlsx: a workbook needs at least one sheet")

// Write renders sheets as an .xlsx package.
//
// Every cell is written as an inline string, including ones that look
// numeric. For a template that is the safe default: it stops Excel from
// coercing an employee number like "00713" into 713, or a version-shaped
// ref like "1.10" into 1.1. The reader converts genuine numbers and dates
// back on the way in.
func Write(w io.Writer, sheets []Sheet) error {
	if len(sheets) == 0 {
		return ErrNoSheets
	}
	if len(sheets) > maxSheets {
		return fmt.Errorf("xlsx: %d sheets exceeds the limit of %d", len(sheets), maxSheets)
	}
	for _, s := range sheets {
		if len(s.Columns) > maxColumns {
			return fmt.Errorf("xlsx: sheet %q has %d columns, limit %d", s.Name, len(s.Columns), maxColumns)
		}
	}

	zw := zip.NewWriter(w)
	add := func(name, body string) error {
		f, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.WriteString(f, body)
		return err
	}

	names := sheetNames(sheets)
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", contentTypesXML(len(sheets))},
		{"_rels/.rels", rootRelsXML()},
		{"xl/workbook.xml", workbookXML(names)},
		{"xl/_rels/workbook.xml.rels", workbookRelsXML(len(sheets))},
		{"xl/styles.xml", stylesXML()},
	}
	for i, s := range sheets {
		parts = append(parts, struct{ name, body string }{
			fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), sheetXML(s),
		})
	}
	for _, p := range parts {
		if err := add(p.name, p.body); err != nil {
			return err
		}
	}
	return zw.Close()
}

// sheetNames sanitises names to what Excel accepts: at most 31 characters,
// none of []:*?/\, non-empty and unique. Excel refuses to open the whole
// package when one name is invalid, so this is enforced rather than
// trusted to callers.
func sheetNames(sheets []Sheet) []string {
	used := make(map[string]bool, len(sheets))
	out := make([]string, len(sheets))
	for i, s := range sheets {
		n := strings.TrimSpace(strings.Map(func(r rune) rune {
			if r < 0x20 || strings.ContainsRune(`[]:*?/\`, r) {
				return '_'
			}
			return r
		}, s.Name))
		if n == "" {
			n = fmt.Sprintf("Sheet%d", i+1)
		}
		base := truncate(n, 31)
		n = base
		for seq := 2; used[strings.ToLower(n)]; seq++ {
			suffix := fmt.Sprintf("~%d", seq)
			n = truncate(base, 31-len(suffix)) + suffix
		}
		used[strings.ToLower(n)] = true
		out[i] = n
	}
	return out
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func contentTypesXML(sheets int) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<Types xmlns="` + nsContent + `">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	b.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	b.WriteString(`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	b.WriteString(`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>`)
	for i := 1; i <= sheets; i++ {
		fmt.Fprintf(&b, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i)
	}
	b.WriteString(`</Types>`)
	return b.String()
}

func rootRelsXML() string {
	return xmlHeader +
		`<Relationships xmlns="` + nsPkgRel + `">` +
		`<Relationship Id="rId1" Type="` + nsRel + `/officeDocument" Target="xl/workbook.xml"/>` +
		`</Relationships>`
}

func workbookXML(names []string) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<workbook xmlns="` + nsMain + `" xmlns:r="` + nsRel + `"><sheets>`)
	for i, n := range names {
		fmt.Fprintf(&b, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, escape(n), i+1, i+1)
	}
	b.WriteString(`</sheets></workbook>`)
	return b.String()
}

func workbookRelsXML(sheets int) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<Relationships xmlns="` + nsPkgRel + `">`)
	for i := 1; i <= sheets; i++ {
		fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="%s/worksheet" Target="worksheets/sheet%d.xml"/>`, i, nsRel, i)
	}
	fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="%s/styles" Target="styles.xml"/>`, sheets+1, nsRel)
	b.WriteString(`</Relationships>`)
	return b.String()
}

// stylesXML declares exactly two cell formats: 0 plain and 1 bold, the
// header style. The two fills and the empty border are not optional
// padding — Excel rejects a styles part whose fill list does not start
// with "none" followed by "gray125".
func stylesXML() string {
	return xmlHeader +
		`<styleSheet xmlns="` + nsMain + `">` +
		`<fonts count="2">` +
		`<font><sz val="11"/><name val="Calibri"/></font>` +
		`<font><b/><sz val="11"/><name val="Calibri"/></font>` +
		`</fonts>` +
		`<fills count="2">` +
		`<fill><patternFill patternType="none"/></fill>` +
		`<fill><patternFill patternType="gray125"/></fill>` +
		`</fills>` +
		`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
		`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
		`<cellXfs count="2">` +
		`<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
		`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/>` +
		`</cellXfs>` +
		`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>` +
		`</styleSheet>`
}

// escape renders s as XML character data. encoding/xml also replaces the
// control characters XML 1.0 forbids, which is what stops a stray byte
// pasted into a spreadsheet from producing a package Excel refuses to
// open with an unhelpful "unreadable content" prompt.
func escape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
