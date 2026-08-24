// Package schema is the contract of the people-and-access import
// workbook: which sheets exist, which columns each carries and what an
// operator is meant to put in them.
//
// It is declared once and used from both directions — the template
// generator writes these columns, the parser reads them back — so a
// change to a header cannot silently break the importer that consumes it.
package schema

// Sheet names. Matching is case-insensitive, so a workbook whose sheets
// were renamed to "people" still imports.
const (
	SheetPeople       = "People"
	SheetGrants       = "Grants"
	SheetMemberships  = "Memberships"
	SheetInstructions = "Instructions"
)

// Column keys, shared by the template writer and the parser.
const (
	ColRealm          = "realm"
	ColUsername       = "username"
	ColEmail          = "email"
	ColCategory       = "category"
	ColExternalRef    = "external_ref"
	ColAssuranceLevel = "assurance_level"

	ColRole         = "role"
	ColScopeAxis    = "scope_axis"
	ColScopeRef     = "scope_ref"
	ColScopeInherit = "scope_inherit"
	ColValidUntil   = "valid_until"
	ColReason       = "reason"

	ColMembership = "membership"
)

// boolValues is the dropdown offered for every true/false column.
var boolValues = []string{"true", "false"}

// Workbook returns the sheets of the import template in order.
//
// There is deliberately no password column. A spreadsheet of plaintext
// passwords is a breach waiting to happen, and it would be mailed around
// as an attachment; imported people are created without a credential and
// get one through the normal invite or reset flow.
func Workbook() []SheetSpec {
	return []SheetSpec{
		{
			Name:    SheetPeople,
			Purpose: "One row per person. Re-running an import leaves people who already exist untouched.",
			Columns: []ColumnSpec{
				{Key: ColRealm, Required: true, Width: 16, Example: "staff",
					Help: "Code of the realm the person belongs to. The realm must already exist."},
				{Key: ColUsername, Required: true, Width: 26, Example: "ada.lovelace",
					Help: "Unique within the realm. This plus realm is what identifies the person on the other sheets."},
				{Key: ColEmail, Width: 30, Example: "ada@example.com",
					Help: "Optional."},
				{Key: ColCategory, Width: 18, Example: "employee",
					Help: "Realm category code. Leave blank for none."},
				{Key: ColExternalRef, Width: 22, Example: "HR-10023",
					Help: "Your own identifier for this person, carried through for reconciliation. Optional."},
				{Key: ColAssuranceLevel, Width: 16, Allowed: []string{"1", "2", "3", "4"}, Example: "1",
					Help: "Identity assurance level. Blank uses the realm's minimum."},
			},
		},
		{
			Name: SheetGrants,
			Purpose: "One row per role a person is granted. Repeat the same person and role on several rows " +
				"to scope one grant to several places.",
			Columns: []ColumnSpec{
				{Key: ColRealm, Required: true, Width: 16, Example: "staff",
					Help: "Realm of the person receiving the grant."},
				{Key: ColUsername, Required: true, Width: 26, Example: "ada.lovelace",
					Help: "The person receiving the grant. They must be on the People sheet or already exist."},
				{Key: ColRole, Required: true, Width: 24, Example: "support-agent",
					Help: "Name of the role to grant. The role must already exist."},
				{Key: ColScopeAxis, Width: 16, Example: "region",
					Help: "Scope axis code. Leave blank to grant the role everywhere the axis allows."},
				{Key: ColScopeRef, Width: 20, Example: "apac",
					Help: "External reference of the scope node. Required when scope_axis is filled in."},
				{Key: ColScopeInherit, Width: 15, Allowed: boolValues, Example: "true",
					Help: "Whether the grant also covers everything beneath that node. Blank means true."},
				{Key: ColValidUntil, Width: 18, Example: "2027-01-31",
					Help: "Date the grant expires, as YYYY-MM-DD. Blank means it does not expire."},
				{Key: ColReason, Width: 28, Example: "onboarding",
					Help: "Recorded in the audit log. Optional."},
			},
		},
		{
			Name:    SheetMemberships,
			Purpose: "One row per person added to a membership. This sheet only ever adds; removing access stays a console action.",
			Columns: []ColumnSpec{
				{Key: ColRealm, Required: true, Width: 16, Example: "staff",
					Help: "Realm of the person joining."},
				{Key: ColUsername, Required: true, Width: 26, Example: "ada.lovelace",
					Help: "The person joining. They must be on the People sheet or already exist."},
				{Key: ColMembership, Required: true, Width: 26, Example: "support-team",
					Help: "Name of the membership to add them to. It must already exist."},
			},
		},
	}
}

// Sheet returns the spec for a sheet name, matched case-insensitively.
func Sheet(name string) (SheetSpec, bool) {
	for _, s := range Workbook() {
		if equalFold(s.Name, name) {
			return s, true
		}
	}
	return SheetSpec{}, false
}
