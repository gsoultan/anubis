// Package provisioningdomain parses and reports on a people-and-access
// import.
//
// It deals in tables — a header row and its data rows — not in
// spreadsheets. Keeping the file format out of the domain is what lets the
// same rules serve an Excel upload today and a CSV or JSON one later
// without a line of this package changing.
package provisioningdomain

// Table is one sheet reduced to its header row and its data rows. Rows may
// be ragged; a row that stops short of a column simply has no value there.
type Table struct {
	Header []string
	Rows   [][]string
}
