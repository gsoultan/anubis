package provisioningapp

// ImportInput is one uploaded workbook.
type ImportInput struct {
	// Data is the .xlsx file exactly as uploaded.
	Data []byte
	// Dry asks what the import would do without changing anything. It is
	// the intended first step: the report it returns lists every problem
	// in the file at once, so the operator fixes them in one pass rather
	// than one upload at a time.
	Dry bool
}
