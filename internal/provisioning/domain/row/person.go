// Package row holds the typed rows an import workbook parses into: one
// type per sheet, carrying the spreadsheet line each came from so every
// problem can be reported at the row an operator can actually go and fix.
package row

// Person is one validated row of the People sheet.
type Person struct {
	Row            int
	Realm          string
	Username       string
	Email          string
	Category       string
	ExternalRef    string
	AssuranceLevel int
}
