package row

import "time"

// Grant is one role granted to one person, together with the scopes it is
// limited to.
//
// Several spreadsheet rows naming the same person, role and expiry
// collapse into a single Grant carrying several scopes. That is what an
// operator listing two regions on two rows means: scopes within an axis
// are OR-ed by authorize(), so one grant over both regions is the same
// decision as two grants — and it is one thing to review and revoke
// later rather than two.
type Grant struct {
	// Rows is every spreadsheet line that folded into this grant, in
	// order, so the report can point at all of them.
	Rows       []int
	Realm      string
	Username   string
	Role       string
	Scopes     []GrantScope
	ValidUntil *time.Time
	Reason     string
}

// Line is the first spreadsheet row that contributed to this grant.
func (g Grant) Line() int {
	if len(g.Rows) == 0 {
		return 0
	}
	return g.Rows[0]
}
