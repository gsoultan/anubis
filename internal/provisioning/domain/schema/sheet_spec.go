package schema

// SheetSpec describes one sheet of the import workbook.
type SheetSpec struct {
	Name    string
	Purpose string
	Columns []ColumnSpec
}

// Keys returns the sheet's column keys in template order.
func (s SheetSpec) Keys() []string {
	out := make([]string, len(s.Columns))
	for i, c := range s.Columns {
		out[i] = c.Key
	}
	return out
}
