package xlsx

import (
	"fmt"
	"strconv"
	"strings"
)

// validationRows is how far down a column a template's dropdown reaches.
// Excel needs a bounded range, and extending it well past the example
// rows is what lets an operator paste in a thousand people and still get
// the dropdown on every one of them.
const validationRows = 5000

// maxInlineList is Excel's limit on an inline list formula, quotes
// included. A longer list silently stops working in some Excel builds, so
// an oversized dropdown is dropped rather than shipped broken.
const maxInlineList = 255

// sheetXML renders one worksheet. Element order is fixed by the
// SpreadsheetML schema — sheetViews, cols, sheetData, autoFilter,
// dataValidations — and Excel enforces it strictly, so this function is
// the one place that order is expressed.
func sheetXML(s Sheet) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<worksheet xmlns="` + nsMain + `" xmlns:r="` + nsRel + `">`)

	// A frozen header row is the single change that makes a wide template
	// usable once the operator scrolls past the first screen.
	b.WriteString(`<sheetViews><sheetView workbookViewId="0">`)
	b.WriteString(`<pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/>`)
	b.WriteString(`</sheetView></sheetViews>`)
	b.WriteString(`<sheetFormatPr defaultRowHeight="15"/>`)

	writeCols(&b, s.Columns)

	b.WriteString(`<sheetData>`)
	if len(s.Columns) > 0 {
		writeRow(&b, 1, s.Header(), true)
	}
	for i, row := range s.Rows {
		writeRow(&b, i+2, row, false)
	}
	b.WriteString(`</sheetData>`)

	if n := len(s.Columns); n > 0 {
		fmt.Fprintf(&b, `<autoFilter ref="A1:%s1"/>`, ColumnName(n-1))
	}
	writeValidations(&b, s.Columns)

	b.WriteString(`</worksheet>`)
	return b.String()
}

func writeCols(b *strings.Builder, cols []Column) {
	if len(cols) == 0 {
		return
	}
	b.WriteString(`<cols>`)
	for i, c := range cols {
		w := c.Width
		if w <= 0 {
			w = defaultWidth
		}
		fmt.Fprintf(b, `<col min="%d" max="%d" width="%s" customWidth="1"/>`,
			i+1, i+1, strconv.FormatFloat(w, 'f', 2, 64))
	}
	b.WriteString(`</cols>`)
}

// writeRow emits one row. Empty data cells are skipped entirely, which is
// how Excel itself stores a sparse row; the reader restores the shape from
// each cell's reference. Header cells are always written so the sheet
// keeps a complete, styled header even where a column has no name.
func writeRow(b *strings.Builder, n int, cells []string, header bool) {
	fmt.Fprintf(b, `<row r="%d">`, n)
	for i, v := range cells {
		if v == "" && !header {
			continue
		}
		if i >= maxColumns {
			break
		}
		style := ""
		if header {
			style = ` s="1"`
		}
		fmt.Fprintf(b, `<c r="%s%d"%s t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`,
			ColumnName(i), n, style, escape(v))
	}
	b.WriteString(`</row>`)
}

// writeValidations turns Column.Allowed into Excel dropdowns. A value
// containing a comma or a double quote cannot be expressed in an inline
// list formula, so such a column is left free-text rather than given a
// dropdown that would silently mangle it — the importer still validates
// the value server-side either way.
func writeValidations(b *strings.Builder, cols []Column) {
	var body strings.Builder
	count := 0
	for i, c := range cols {
		list, ok := inlineList(c.Allowed)
		if !ok {
			continue
		}
		ref := ColumnName(i)
		fmt.Fprintf(&body,
			`<dataValidation type="list" allowBlank="1" showInputMessage="1" showErrorMessage="1" sqref="%s2:%s%d">`+
				`<formula1>"%s"</formula1></dataValidation>`,
			ref, ref, validationRows, escape(list))
		count++
	}
	if count > 0 {
		fmt.Fprintf(b, `<dataValidations count="%d">%s</dataValidations>`, count, body.String())
	}
}

func inlineList(allowed []string) (string, bool) {
	if len(allowed) == 0 {
		return "", false
	}
	for _, a := range allowed {
		if a == "" || strings.ContainsAny(a, `",`) {
			return "", false
		}
	}
	list := strings.Join(allowed, ",")
	if len(list)+2 > maxInlineList {
		return "", false
	}
	return list, true
}
