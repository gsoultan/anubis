package usecase

// SyncRowInput is one row of a structure feed: {ref, parent_ref, name, type}.
type SyncRowInput struct {
	Ref       string `json:"ref"`
	ParentRef string `json:"parent_ref,omitempty"`
	Name      string `json:"name"`
	NodeType  string `json:"node_type,omitempty"`
}
