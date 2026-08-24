package xlsx

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"strconv"
	"strings"
	"time"

	"archive/zip"
)

const (
	// maxPartBytes bounds the decompressed size of any single XML part.
	// A workbook arriving over an RPC is attacker-supplied, and a zip bomb
	// is a handful of bytes on the wire, so both the declared size and the
	// bytes actually produced are checked.
	maxPartBytes = 64 << 20

	// maxRowsPerSheet bounds one sheet. Exceeding it is an error rather
	// than a truncation: silently importing the first N rows of a larger
	// file would look like success and lose the rest.
	maxRowsPerSheet = 200_000
)

// ErrNotWorkbook is returned when the bytes are not a readable .xlsx —
// wrong format, or a package with no workbook part.
var ErrNotWorkbook = errors.New("xlsx: not a readable .xlsx workbook")

// Read parses an .xlsx package into sheets, in workbook order.
//
// It resolves all three ways Excel stores text (shared table, inline,
// formula result), restores sparse rows and cells from their references,
// and converts date-formatted serial numbers back to ISO-8601. That last
// part is not a nicety: a date typed into a template is stored as a bare
// number like 46418, so without it every date column imports as nonsense.
func Read(r io.ReaderAt, size int64) ([]Sheet, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotWorkbook, err)
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[path.Clean(f.Name)] = f
	}

	var wb xlWorkbook
	if err := decodePart(files, "xl/workbook.xml", &wb); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotWorkbook, err)
	}
	if len(wb.Sheets) == 0 {
		return nil, ErrNotWorkbook
	}
	if len(wb.Sheets) > maxSheets {
		return nil, fmt.Errorf("xlsx: %d sheets exceeds the limit of %d", len(wb.Sheets), maxSheets)
	}

	targets := map[string]string{}
	var rels xlRels
	// A workbook with a single sheet and no rels part is unusual but
	// legal; fall back to positional sheet parts rather than failing.
	if err := decodePart(files, "xl/_rels/workbook.xml.rels", &rels); err == nil {
		for _, rel := range rels.Rels {
			targets[rel.ID] = resolveTarget("xl", rel.Target)
		}
	}

	sst, err := sharedStrings(files)
	if err != nil {
		return nil, err
	}
	dateStyles, err := dateStyleSet(files)
	if err != nil {
		return nil, err
	}

	sheets := make([]Sheet, 0, len(wb.Sheets))
	for i, ref := range wb.Sheets {
		name := targets[ref.RID]
		if name == "" {
			name = fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1)
		}
		f, ok := files[name]
		if !ok {
			// A sheet listed in the workbook whose part is missing is a
			// damaged package, not an empty sheet — but one bad sheet
			// should not cost the operator the other two.
			sheets = append(sheets, Sheet{Name: ref.Name})
			continue
		}
		rows, err := parseSheet(f, sst, dateStyles)
		if err != nil {
			return nil, fmt.Errorf("xlsx: sheet %q: %w", ref.Name, err)
		}
		sheets = append(sheets, toSheet(ref.Name, rows))
	}
	return sheets, nil
}

// toSheet splits a raw grid into the header row and the data rows, the
// mirror of what Write does, so a workbook survives a round trip.
func toSheet(name string, rows [][]string) Sheet {
	s := Sheet{Name: name}
	if len(rows) == 0 {
		return s
	}
	s.Columns = make([]Column, len(rows[0]))
	for i, h := range rows[0] {
		s.Columns[i] = Column{Header: h}
	}
	if len(rows) > 1 {
		s.Rows = rows[1:]
	}
	return s
}

func resolveTarget(base, target string) string {
	if strings.HasPrefix(target, "/") {
		return path.Clean(strings.TrimPrefix(target, "/"))
	}
	return path.Clean(path.Join(base, target))
}

func partBytes(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > maxPartBytes {
		return nil, fmt.Errorf("part %q declares %d bytes, limit %d",
			f.Name, f.UncompressedSize64, maxPartBytes)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	// The declared size is part of the same untrusted file, so the bytes
	// actually produced are bounded too.
	b, err := io.ReadAll(io.LimitReader(rc, maxPartBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxPartBytes {
		return nil, fmt.Errorf("part %q exceeds the %d byte limit", f.Name, maxPartBytes)
	}
	return b, nil
}

func decodePart(files map[string]*zip.File, name string, v any) error {
	f, ok := files[name]
	if !ok {
		return fmt.Errorf("missing part %q", name)
	}
	b, err := partBytes(f)
	if err != nil {
		return err
	}
	return xml.Unmarshal(b, v)
}

func sharedStrings(files map[string]*zip.File) ([]string, error) {
	var sst xlSST
	if err := decodePart(files, "xl/sharedStrings.xml", &sst); err != nil {
		// Writers that use only inline strings omit the part entirely.
		return nil, nil
	}
	out := make([]string, len(sst.Items))
	for i, si := range sst.Items {
		out[i] = si.value()
	}
	return out, nil
}

// builtinDateFormats are the number format ids Excel reserves for dates
// and times. They are not written into styles.xml — a cell just points at
// id 14 and every reader is expected to know it means a short date.
var builtinDateFormats = map[int]bool{
	14: true, 15: true, 16: true, 17: true, 18: true, 19: true, 20: true,
	21: true, 22: true, 27: true, 28: true, 29: true, 30: true, 31: true,
	32: true, 33: true, 34: true, 35: true, 36: true, 45: true, 46: true,
	47: true, 50: true, 51: true, 52: true, 53: true, 54: true, 55: true,
	56: true, 57: true, 58: true,
}

// dateStyleSet resolves which cellXfs indices format their value as a
// date, so a numeric cell can be told apart from a date cell — in the
// file they are the same number.
func dateStyleSet(files map[string]*zip.File) (map[int]bool, error) {
	var st xlStyles
	if err := decodePart(files, "xl/styles.xml", &st); err != nil {
		return map[int]bool{}, nil
	}
	custom := make(map[int]bool, len(st.NumFmts))
	for _, nf := range st.NumFmts {
		custom[nf.ID] = isDateFormat(nf.Code)
	}
	out := make(map[int]bool, len(st.Xfs))
	for i, xf := range st.Xfs {
		if builtinDateFormats[xf.NumFmtID] || custom[xf.NumFmtID] {
			out[i] = true
		}
	}
	return out, nil
}

// isDateFormat reports whether a custom format code renders a date. Date
// field characters are searched for only outside quoted literals, bracket
// sections and backslash escapes, so a currency format like "d"#,##0 is
// not mistaken for a day.
func isDateFormat(code string) bool {
	rs := []rune(code)
	inQuote, inBracket := false, false
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case inQuote:
			if c == '"' {
				inQuote = false
			}
		case inBracket:
			if c == ']' {
				inBracket = false
			}
		case c == '"':
			inQuote = true
		case c == '[':
			inBracket = true
		case c == '\\':
			i++
		case c == 'y', c == 'Y', c == 'd', c == 'D', c == 'h', c == 'H',
			c == 's', c == 'S', c == 'm', c == 'M':
			return true
		}
	}
	return false
}

// excelEpoch is 1899-12-30 rather than 1900-01-01 because Excel
// reproduces a Lotus 1-2-3 bug that counts 1900 as a leap year. Shifting
// the epoch back two days cancels the phantom 29 February for every date
// from 1900-03-01 on, which covers anything a person types into an
// import template.
var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

func serialToString(f float64) string {
	days := math.Floor(f)
	// Spreadsheet times are binary fractions of a day and land a hair
	// under the value a human typed; rounding to the second restores it.
	secs := int(math.Round((f - days) * 24 * 60 * 60))
	t := excelEpoch.AddDate(0, 0, int(days)).Add(time.Duration(secs) * time.Second)
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
		return t.Format("2006-01-02")
	}
	return t.Format(time.RFC3339)
}

func parseSheet(f *zip.File, sst []string, dateStyles map[int]bool) ([][]string, error) {
	if f.UncompressedSize64 > maxPartBytes {
		return nil, fmt.Errorf("part %q declares %d bytes, limit %d",
			f.Name, f.UncompressedSize64, maxPartBytes)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	dec := xml.NewDecoder(io.LimitReader(rc, maxPartBytes+1))
	var rows [][]string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "row" {
			continue
		}
		var r xlRow
		if err := dec.DecodeElement(&r, &se); err != nil {
			return nil, err
		}
		// Excel omits empty rows, so r carries the true row number; pad
		// the gap to keep every row's position honest. Row numbers are
		// what an import report points an operator at.
		if r.R > 0 && r.R > len(rows)+1 {
			if r.R > maxRowsPerSheet {
				return nil, fmt.Errorf("row %d exceeds the %d row limit", r.R, maxRowsPerSheet)
			}
			for len(rows) < r.R-1 {
				rows = append(rows, nil)
			}
		}
		if len(rows) >= maxRowsPerSheet {
			return nil, fmt.Errorf("sheet exceeds the %d row limit", maxRowsPerSheet)
		}
		rows = append(rows, rowValues(r, sst, dateStyles))
	}
	// Excel keeps styled-but-empty rows past the data; they are not rows
	// an operator meant to import.
	for len(rows) > 0 && isBlank(rows[len(rows)-1]) {
		rows = rows[:len(rows)-1]
	}
	return rows, nil
}

func rowValues(r xlRow, sst []string, dateStyles map[int]bool) []string {
	// Keyed by a column index the file chose, and bounded by columnIndex
	// rejecting anything past Excel's own column limit.
	cells := make(map[int]string, len(r.Cells))
	width := 0
	for _, c := range r.Cells {
		i := columnIndex(c.R)
		if i < 0 {
			continue
		}
		cells[i] = cellValue(c, sst, dateStyles)
		if i+1 > width {
			width = i + 1
		}
	}
	if width == 0 {
		return nil
	}
	out := make([]string, width)
	for i := range out {
		out[i] = cells[i]
	}
	return out
}

func cellValue(c xlCell, sst []string, dateStyles map[int]bool) string {
	v := strings.TrimSpace(c.V)
	switch c.T {
	case "s":
		i, err := strconv.Atoi(v)
		if err != nil || i < 0 || i >= len(sst) {
			return ""
		}
		return sst[i]
	case "inlineStr":
		if c.IS == nil {
			return ""
		}
		return c.IS.value()
	case "str", "e":
		return c.V
	case "b":
		if v == "1" {
			return "true"
		}
		return "false"
	default:
		if v == "" {
			return ""
		}
		if dateStyles[c.S] {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
				return serialToString(f)
			}
		}
		return v
	}
}

func isBlank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
