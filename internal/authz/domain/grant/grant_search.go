package grant

// GrantSearch narrows the Access screen. Every field is optional; together
// they are what makes a directory of hundreds of thousands of grants
// answerable.
type GrantSearch struct {
	Query      string
	IdentityID string
	RoleID     string
	// Source is "direct", "membership", or empty for both.
	Source         string
	IncludeRevoked bool
	// Cursor is the id of the last row of the previous page.
	Cursor   string
	PageSize int
}

// GrantHit is one search result: the grant plus the name of the person who
// holds it, so the console does not resolve one lookup per line.
type GrantHit struct {
	Grant    GrantRecord
	Username string
}
