package repository

type GrantScopeRecord struct {
	GrantID  string
	Axis     string
	NodeID   string
	NodeName string
	Inherit  bool
}
