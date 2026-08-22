package usecase

// Explanation carries the verdict plus the decomposition as JSON produced by
// authorize_explain() — identity gates, permission gates, per-grant per-axis
// satisfaction, provenance.
type Explanation struct {
	Allow       bool
	Reason      string
	FailingAxis string
	DetailJSON  string
}
