package provisioningdomain

// RowIssue is one thing wrong with one row, addressed the way the operator
// sees their own file: the sheet name, the spreadsheet row number, and the
// column header. A Row of zero means the problem is with the sheet as a
// whole rather than any single line.
type RowIssue struct {
	Sheet   string `json:"sheet"`
	Row     int    `json:"row,omitempty"`
	Column  string `json:"column,omitempty"`
	Message string `json:"message"`
}
