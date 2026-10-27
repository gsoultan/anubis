package snapshot

import "testing"

// A share lets a recipient through on ONE record, for the permissions it
// names, while it lasts — and nothing else about the gate changes: the
// identity and permission gates still apply, grants still win on their own,
// and a call that names no record never consults a share.
func TestASharedRecordAllowsOnlyThatRecordAndThosePermissions(t *testing.T) {
	d := grantedProbe()
	d.Permissions["training:request:comment"] = Permission{Key: "training:request:comment"}
	d.SharesByIdentity = map[string][]Share{
		subject: {{
			Record:      "job_requisition/JR-0142",
			Permissions: map[string]bool{"training:request:approve": true},
			ValidUntil:  at(now + 3600),
		}},
		"blocked": {{
			Record: "job_requisition/JR-0142", Permissions: map[string]bool{perm: true}, ValidUntil: at(now + 3600),
		}},
	}
	rec := map[string]string{"_record": "job_requisition/JR-0142"}

	if !d.Evaluate(subject, perm, rec, at(now)) {
		t.Fatal("the shared record was refused")
	}
	if d.Evaluate(subject, perm, map[string]string{"_record": "job_requisition/JR-0001"}, at(now)) {
		t.Fatal("a share on one record opened another")
	}
	if d.Evaluate(subject, "training:request:comment", rec, at(now)) {
		t.Fatal("a share conferred a permission it does not name")
	}
	if d.Evaluate(subject, perm, nil, at(now)) {
		t.Fatal("a call that names no record was let through by a share")
	}
	if d.Evaluate(subject, perm, rec, at(now+3601)) {
		t.Fatal("a lapsed share still allowed")
	}
	if d.Evaluate("blocked", perm, rec, at(now)) {
		t.Fatal("a blocked identity was let through by a share")
	}
	if d.Evaluate(subject, "no:such:permission", rec, at(now)) {
		t.Fatal("an unknown permission was let through by a share")
	}
}

// A grant that reaches the target still decides first; a share does not
// make a held place unheld, and a place-bound call with a record named is
// answered by the grant when it covers the place.
func TestAGrantStillWinsWhenARecordIsNamed(t *testing.T) {
	d := withGrants(grantedProbe(),
		Grant{ID: "g1", RoleID: role, Scopes: map[string][]ScopeConstraint{"org": {{NodeID: "finance", Inherit: true}}}},
	)
	targets := map[string]string{"org": "finance", "_record": "job_requisition/JR-9"}
	if !d.Evaluate(subject, perm, targets, at(now)) {
		t.Fatal("a covering grant was refused because a record was named")
	}
	targets["org"] = "hr"
	if d.Evaluate(subject, perm, targets, at(now)) {
		t.Fatal("an uncovered place was allowed with no share")
	}
}

// Shares are not places: GrantedNodes and Reaches ignore them, so a shared
// record cannot widen a list filter or a scope switch.
func TestSharesDoNotReachPlaces(t *testing.T) {
	d := grantedProbe()
	d.SharesByIdentity = map[string][]Share{subject: {{
		Record: "x/1", Permissions: map[string]bool{perm: true}, ValidUntil: at(now + 3600),
	}}}
	out, known := d.GrantedNodes(subject, perm, "org", nil, at(now))
	if !known || out.Everywhere || out.OwnRecords || len(out.Nodes) != 0 {
		t.Fatalf("a share leaked into the place list: %+v", out)
	}
	if d.Reaches(subject, "org", "finance", at(now)) {
		t.Fatal("a share let the subject switch to a place")
	}
}
