package xlsx

// Column describes one worksheet column: the header text, how wide Excel
// should render it, and optionally the closed set of values offered as a
// dropdown. Allowed drives a list dataValidation, which is what turns a
// generated template into a self-documenting form rather than a bare
// header row an operator has to guess at.
type Column struct {
	Header  string
	Width   float64
	Allowed []string
}
