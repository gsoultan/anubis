package provisioningapp

import (
	"bytes"
	"strconv"

	"github.com/gsoultan/anubis/internal/platform/xlsx"
	"github.com/gsoultan/anubis/internal/provisioning/domain/schema"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// TemplateFilename is what the workbook is offered to the operator as.
const TemplateFilename = "anubis-people-and-access.xlsx"

// renderTemplate builds the import workbook from the schema, so the
// columns handed out are by construction the columns the parser reads
// back. The Instructions sheet is generated from the same specs, which is
// why the help text cannot go stale.
func renderTemplate() ([]byte, error) {
	specs := schema.Workbook()
	sheets := make([]xlsx.Sheet, 0, len(specs)+1)
	sheets = append(sheets, instructionsSheet(specs))
	for _, spec := range specs {
		cols := make([]xlsx.Column, len(spec.Columns))
		for i, c := range spec.Columns {
			cols[i] = xlsx.Column{Header: c.Key, Width: c.Width, Allowed: c.Allowed}
		}
		// The data sheets ship empty on purpose. A sample row left in by
		// an operator who did not notice it is a fictional person created
		// in a real directory; the Instructions sheet carries the example
		// for every column instead, where it cannot be imported.
		sheets = append(sheets, xlsx.Sheet{Name: spec.Name, Columns: cols})
	}

	var buf bytes.Buffer
	if err := xlsx.Write(&buf, sheets); err != nil {
		return nil, apperr.ErrInternal.Wrap(err)
	}
	return buf.Bytes(), nil
}

func instructionsSheet(specs []schema.SheetSpec) xlsx.Sheet {
	rows := [][]string{
		{"", "", "", ""},
		{"How to use this workbook", "", "", ""},
		{"", "", "", ""},
		{"1.", "Fill in the sheets below. Use only the ones you need — a workbook of people alone is fine. Every column is described further down this page.", "", ""},
		{"2.", "Upload it in the console and choose Check first. Nothing is written; you get back a list of everything wrong with the file.", "", ""},
		{"3.", "Fix what it reports and upload again. An import applies in full or not at all.", "", ""},
		{"", "", "", ""},
		{"Re-running an import is safe.", "People who already exist are left alone, roles somebody already holds are not granted twice.", "", ""},
		{"Nobody gets a password here.", "Imported people are created without one and get it through the normal invite or reset flow.", "", ""},
		{"", "", "", ""},
	}
	for _, spec := range specs {
		rows = append(rows,
			[]string{spec.Name, spec.Purpose, "", ""},
			[]string{"column", "required", "example", "what goes in it"},
		)
		for _, c := range spec.Columns {
			rows = append(rows, []string{c.Key, yesNo(c.Required), c.Example, c.Help})
		}
		rows = append(rows, []string{"", "", "", ""})
	}
	return xlsx.Sheet{
		Name: schema.SheetInstructions,
		Columns: []xlsx.Column{
			{Header: "Anubis import template", Width: 30},
			{Header: "", Width: 14},
			{Header: "", Width: 22},
			{Header: "", Width: 90},
		},
		Rows: rows,
	}
}

func yesNo(b bool) string {
	if b {
		return "required"
	}
	return "optional"
}

func itoa(i int) string { return strconv.Itoa(i) }
