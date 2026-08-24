package provisioningdomain

// MaxReportIssues bounds how many individual problems a report carries.
// A workbook where every row is wrong would otherwise produce a report
// far larger than the file that caused it.
const MaxReportIssues = 500

// ImportReport is what an operator gets back from an import, and the only
// thing they get back from a dry run. The counters are deliberately split
// between what changed and what was already true: re-running an import is
// a normal thing to do, and "0 created, 812 already there" is the answer
// that tells an operator it worked.
type ImportReport struct {
	Dry bool `json:"dry"`
	// Applied is true only when the work was committed. A dry run and an
	// import refused over its issues both come back with counters filled
	// in and Applied false — without this flag those two are
	// indistinguishable from "done".
	Applied bool `json:"applied"`

	PeopleCreated  int `json:"people_created"`
	PeopleExisting int `json:"people_existing"`

	GrantsCreated int `json:"grants_created"`
	GrantsSkipped int `json:"grants_skipped"`

	MembershipsAssigned int `json:"memberships_assigned"`
	MembershipsExisting int `json:"memberships_existing"`

	Issues []RowIssue `json:"issues,omitempty"`
	// IssuesOmitted counts problems past MaxReportIssues. It is reported
	// rather than dropped so a truncated report never reads as a clean one.
	IssuesOmitted int `json:"issues_omitted,omitempty"`
}

// AddIssue records a problem, keeping the report bounded.
func (r *ImportReport) AddIssue(i RowIssue) {
	if len(r.Issues) >= MaxReportIssues {
		r.IssuesOmitted++
		return
	}
	r.Issues = append(r.Issues, i)
}

// AddIssues records many problems.
func (r *ImportReport) AddIssues(is []RowIssue) {
	for _, i := range is {
		r.AddIssue(i)
	}
}

// OK reports whether the import found nothing to complain about.
func (r ImportReport) OK() bool {
	return len(r.Issues) == 0 && r.IssuesOmitted == 0
}

// Changed is the number of things the import did, or would do.
func (r ImportReport) Changed() int {
	return r.PeopleCreated + r.GrantsCreated + r.MembershipsAssigned
}
