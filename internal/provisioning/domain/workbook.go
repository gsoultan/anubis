package provisioningdomain

import "github.com/gsoultan/anubis/internal/provisioning/domain/row"

// Workbook is a parsed import: the rows that survived structural
// validation. Rows that did not are RowIssues instead, so a Workbook only
// ever holds work that is worth attempting.
type Workbook struct {
	People      []row.Person
	Grants      []row.Grant
	Memberships []row.Membership
}

// Len is the number of actions the workbook describes.
func (w Workbook) Len() int {
	return len(w.People) + len(w.Grants) + len(w.Memberships)
}
