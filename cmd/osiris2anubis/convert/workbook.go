package convert

import (
	"io"

	"github.com/gsoultan/anubis/internal/platform/xlsx"
	"github.com/gsoultan/anubis/internal/provisioning/domain/schema"
)

// WriteWorkbook writes the people-and-access workbook the ProvisioningService
// imports: the sheets and columns come from the import's own schema, so a
// column renamed there cannot leave this writer producing a file the
// importer no longer reads.
func WriteWorkbook(w io.Writer, r PeopleResult) error {
	rows := map[string][][]string{
		schema.SheetPeople: r.People,
		schema.SheetGrants: r.Grants,
	}
	var sheets []xlsx.Sheet
	for _, spec := range schema.Workbook() {
		cols := make([]xlsx.Column, len(spec.Columns))
		for i, c := range spec.Columns {
			cols[i] = xlsx.Column{Header: c.Key, Width: c.Width}
		}
		sheets = append(sheets, xlsx.Sheet{Name: spec.Name, Columns: cols, Rows: rows[spec.Name]})
	}
	return xlsx.Write(w, sheets)
}
