package repository

// SyncFeedRow is one row of an external structure feed, keyed on the
// idempotent external_ref (parents must precede children).
type SyncFeedRow struct {
	Ref       string `json:"ref"`
	ParentRef string `json:"parent_ref,omitempty"`
	Name      string `json:"name"`
	NodeType  string `json:"node_type,omitempty"`
}
