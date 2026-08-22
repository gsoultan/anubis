package scopedomain

type ScopeAxisRecord struct {
	Code          string
	DisplayName   string
	DefaultEffect string
	Status        string
	SortOrder     int
	Resolution    []byte
	UISchema      []byte
}
