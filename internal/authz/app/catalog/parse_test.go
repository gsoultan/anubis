package authzcatalog

import (
	"strings"
	"testing"
)

// An absent section and an empty one are different instructions, and the
// apply path does very different things with them. Nothing else in this
// package matters as much.
func TestJSONSectionPresence(t *testing.T) {
	cases := []struct {
		name                       string
		in                         string
		perms, roles, routes       bool
		permCount, roleCount, rtCt int
	}{
		{name: "empty object declares nothing", in: `{}`},
		{
			name: "explicit empty array is a declaration",
			in:   `{"routes":[]}`, routes: true,
		},
		{
			name:  "roles only leaves permissions alone",
			in:    `{"roles":[{"name":"viewer"}]}`,
			roles: true, roleCount: 1,
		},
		{
			name: "all three",
			in: `{"permissions":[{"resource":"invoice","action":"read"}],
			      "roles":[{"name":"viewer"}],
			      "routes":[{"priority":1,"path_pattern":"/x","effect":"public"}]}`,
			perms: true, roles: true, routes: true,
			permCount: 1, roleCount: 1, rtCt: 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := Parse(c.in, "")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if d.HasPermissions != c.perms || d.HasRoles != c.roles || d.HasRoutes != c.routes {
				t.Fatalf("presence = %v/%v/%v, want %v/%v/%v",
					d.HasPermissions, d.HasRoles, d.HasRoutes, c.perms, c.roles, c.routes)
			}
			if len(d.Permissions) != c.permCount || len(d.Roles) != c.roleCount || len(d.Routes) != c.rtCt {
				t.Fatalf("counts = %d/%d/%d, want %d/%d/%d",
					len(d.Permissions), len(d.Roles), len(d.Routes), c.permCount, c.roleCount, c.rtCt)
			}
		})
	}
}

func TestCSVPermissions(t *testing.T) {
	// Deliberately awkward: a BOM from Excel, mixed header spelling, a blank
	// line in the middle, a short final row, and a quoted multi-value cell.
	in := "\ufeff" + `Resource,Action,Description,Risk,Min Assurance,requires-amr
invoice,read,Read invoices,,1,
invoice,approve,Approve an invoice,critical,2,"pwd,otp"

invoice,delete
`
	d, err := Parse(in, FormatCSV)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !d.HasPermissions {
		t.Fatal("permissions section not declared")
	}
	if d.HasRoles || d.HasRoutes {
		t.Fatal("a permissions CSV must say nothing about roles or routes")
	}
	if len(d.Permissions) != 3 {
		t.Fatalf("rows = %d, want 3 (the blank line is not a permission)", len(d.Permissions))
	}
	if got := d.Permissions[0].Risk; got != "normal" {
		t.Errorf("blank risk = %q, want the table's own default", got)
	}
	if got := d.Permissions[1].RequiresAMR; len(got) != 2 || got[0] != "pwd" || got[1] != "otp" {
		t.Errorf("requires_amr = %v, want [pwd otp]", got)
	}
	if got := d.Permissions[1].MinAssurance; got != 2 {
		t.Errorf("min_assurance = %d, want 2", got)
	}
	// The short row is the Excel export that dropped its empty trailing cells.
	if got := d.Permissions[2]; got.Resource != "invoice" || got.Action != "delete" {
		t.Errorf("short row = %+v, want invoice:delete", got)
	}
}

func TestCSVRoles(t *testing.T) {
	in := `role,description,permissions,patterns,allowed_realm_kinds
approver,Approves invoices,"invoice:read;invoice:approve",,internal;partner
viewer,Reads them,"invoice:read",invoice:*,internal
`
	d, err := Parse(in, FormatCSV)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !d.HasRoles || d.HasPermissions {
		t.Fatal("a roles CSV declares roles and nothing else")
	}
	if len(d.Roles) != 2 {
		t.Fatalf("roles = %d, want 2", len(d.Roles))
	}
	if got := d.Roles[0].Permissions; len(got) != 2 || got[1] != "invoice:approve" {
		t.Errorf("semicolon list = %v", got)
	}
	if got := d.Roles[0].AllowedRealmKinds; len(got) != 2 || got[0] != "internal" {
		t.Errorf("realm kinds = %v", got)
	}
	if got := d.Roles[1].Patterns; len(got) != 1 || got[0] != "invoice:*" {
		t.Errorf("patterns = %v", got)
	}
}

// A file whose header names neither sheet has to say what both look like:
// the operator is holding an export from a system that knows nothing about
// this schema, and guessing at it is the whole task.
func TestCSVUnknownHeader(t *testing.T) {
	_, err := Parse("id,label,owner\n1,x,y\n", FormatCSV)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	msg := err.Error()
	for _, want := range []string{"resource", "role"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message does not mention the %q sheet: %s", want, msg)
		}
	}
}

func TestParseRejects(t *testing.T) {
	cases := []struct{ name, text, format string }{
		{"unknown format", `{}`, "yaml"},
		{"invalid json", `{"roles":`, FormatJSON},
		{"empty csv", ``, FormatCSV},
		{"min_assurance not a number", "resource,action,min_assurance\na,b,high\n", FormatCSV},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Parse(c.text, c.format); err == nil {
				t.Fatal("expected a refusal")
			}
		})
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		doc  Document
		ok   bool
	}{
		{
			name: "clean document",
			doc: Document{
				Permissions: []Permission{{Resource: "invoice", Action: "read", Risk: "normal"}},
				Roles:       []Role{{Name: "viewer"}},
				Routes:      []Route{{Priority: 1, PathPattern: "/a", Effect: "public"}},
			},
			ok: true,
		},
		{
			name: "same permission twice",
			doc: Document{Permissions: []Permission{
				{Resource: "invoice", Action: "read"},
				{Resource: "invoice", Action: "read", Description: "merged from another sheet"},
			}},
		},
		{
			name: "same role twice",
			doc:  Document{Roles: []Role{{Name: "viewer"}, {Name: "viewer"}}},
		},
		{
			name: "missing action",
			doc:  Document{Permissions: []Permission{{Resource: "invoice"}}},
		},
		{
			name: "risk that is not a risk",
			doc:  Document{Permissions: []Permission{{Resource: "a", Action: "b", Risk: "high"}}},
		},
		{
			name: "assurance above the ladder",
			doc:  Document{Permissions: []Permission{{Resource: "a", Action: "b", MinAssurance: 4}}},
		},
		{
			name: "two routes at one priority",
			doc: Document{Routes: []Route{
				{Priority: 1, PathPattern: "/a", Effect: "public"},
				{Priority: 1, PathPattern: "/b", Effect: "deny"},
			}},
		},
		{
			name: "effect that is not an effect",
			doc:  Document{Routes: []Route{{Priority: 1, PathPattern: "/a", Effect: "allow"}}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.doc.Validate()
			if c.ok && err != nil {
				t.Fatalf("want accepted, got %v", err)
			}
			if !c.ok && err == nil {
				t.Fatal("want refused, got accepted")
			}
		})
	}
}

func TestSections(t *testing.T) {
	d := Document{HasPermissions: true, HasRoutes: true}
	got := strings.Join(d.Sections(), ",")
	if got != "permissions,routes" {
		t.Fatalf("sections = %q", got)
	}
}
