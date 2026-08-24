package xlsx

// Sheet is one worksheet: a header row described by Columns followed by
// Rows of already-stringified cells. Write emits Columns as row 1; Read
// fills Columns from row 1 and Rows from the rest, so a workbook survives
// a write/read round trip with its header intact.
//
// Rows may be ragged. Write pads a short row with empty cells and Read
// pads a sparse row back out to the widest cell it saw, so callers never
// have to bounds-check a row against its header.
type Sheet struct {
	Name    string
	Columns []Column
	Rows    [][]string
}

// Header returns the column headers as a plain slice.
func (s Sheet) Header() []string {
	out := make([]string, len(s.Columns))
	for i, c := range s.Columns {
		out[i] = c.Header
	}
	return out
}
