package repository

type ScopeNodeRecord struct {
	ID          string
	Axis        string
	NodeType    string
	ParentID    string
	Slug        string
	Name        string
	ExternalRef string
	Status      string
	IsAxisRoot  bool
}
