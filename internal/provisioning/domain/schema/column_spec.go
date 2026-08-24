package schema

// ColumnSpec describes one column of the import workbook. Key is both the
// header text written into the template and the name the parser looks the
// column up by, which is what keeps a downloaded template and the importer
// that reads it back from ever drifting apart.
type ColumnSpec struct {
	Key      string
	Required bool
	Width    float64
	// Allowed is a closed value set, rendered as an Excel dropdown. It is
	// a usability aid only — the importer revalidates server-side, since
	// nothing stops an operator pasting over a validated cell.
	Allowed []string
	Example string
	Help    string
}
