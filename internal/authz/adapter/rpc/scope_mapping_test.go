package authzrpc

import (
	"reflect"
	"testing"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/internal/authz/domain/grant"
	"github.com/gsoultan/anubis/internal/authz/domain/membership"
)

// A scope pin crosses the wire twice and both crossings are hand-written
// field lists. 0045 proved what that costs: interval_seconds was mapped
// outbound by a shared helper and inbound by four copies, one of which simply
// did not name it. No error, no log — the drawer said "Source connected" and
// the source never ran once.
//
// The field that would go missing here is `exclude`, and it fails in the
// opposite direction to a schedule that never fires: the carve-out vanishes
// and the grant is stored WIDER than the operator asked for. Nothing downstream
// can tell that apart from a grant they meant to write.
//
// So these tests do not check `exclude` specifically. They fill every field of
// the source with a distinct non-zero value and fail if ANY field of the
// destination is left zero — which makes adding a sixth field and forgetting
// to map it a test failure rather than a widening.

// fillDistinct sets every settable string/bool field of the struct behind p to
// a non-zero value, so a mapper that drops one leaves an observable hole.
func fillDistinct(t *testing.T, p any) {
	t.Helper()
	v := reflect.ValueOf(p).Elem()
	filled := 0
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue // protoimpl bookkeeping; not part of the message
		}
		switch f.Kind() {
		case reflect.String:
			f.SetString(v.Type().Field(i).Name + "-value")
			filled++
		case reflect.Bool:
			f.SetBool(true)
			filled++
		default:
			t.Fatalf("%s.%s is a %s — teach fillDistinct about it, do not skip it",
				v.Type(), v.Type().Field(i).Name, f.Kind())
		}
	}
	if filled == 0 {
		t.Fatalf("%s: nothing was filled, the test would pass vacuously", v.Type())
	}
}

// zeroFields names every string/bool field left at its zero value.
func zeroFields(t *testing.T, p any) []string {
	t.Helper()
	v := reflect.ValueOf(p)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	var out []string
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue
		}
		if (f.Kind() == reflect.String && f.String() == "") ||
			(f.Kind() == reflect.Bool && !f.Bool()) {
			out = append(out, v.Type().Field(i).Name)
		}
	}
	return out
}

func TestGrantScopeMapsOutboundWithNoFieldDropped(t *testing.T) {
	var rec grant.GrantScopeRecord
	fillDistinct(t, &rec)

	got := grantScopeProtos([]grant.GrantScopeRecord{rec}, rec.GrantID)
	if len(got) != 1 {
		t.Fatalf("got %d protos, want 1", len(got))
	}
	if missing := zeroFields(t, got[0]); len(missing) > 0 {
		t.Errorf("GrantScope fields left unset by grantScopeProtos: %v", missing)
	}
	if got[0].Axis != rec.Axis || got[0].NodeId != rec.NodeID || got[0].NodeName != rec.NodeName {
		t.Errorf("fields crossed over: %+v from %+v", got[0], rec)
	}
}

func TestEntryScopeMapsOutboundWithNoFieldDropped(t *testing.T) {
	var rec membership.MembershipEntryScopeRecord
	fillDistinct(t, &rec)

	got := entryScopeProtos([]membership.MembershipEntryScopeRecord{rec}, rec.EntryID)
	if len(got) != 1 {
		t.Fatalf("got %d protos, want 1", len(got))
	}
	if missing := zeroFields(t, got[0]); len(missing) > 0 {
		t.Errorf("GrantScope fields left unset by entryScopeProtos: %v", missing)
	}
}

func TestGrantScopeMapsInboundWithNoFieldDropped(t *testing.T) {
	msg := &anubisv1.GrantScope{}
	fillDistinct(t, msg)

	got := scopeInputs([]*anubisv1.GrantScope{msg})
	if len(got) != 1 {
		t.Fatalf("got %d inputs, want 1", len(got))
	}
	if missing := zeroFields(t, &got[0]); len(missing) > 0 {
		t.Errorf("GrantScopeInput fields left unset by scopeInputs: %v", missing)
	}
	if got[0].Axis != msg.Axis || got[0].NodeID != msg.NodeId {
		t.Errorf("fields crossed over: %+v from %+v", got[0], msg)
	}
}

// The wire default has to be the safe reading. A client built before
// exclusions exist sends no `exclude` at all; proto3 hands us false, and false
// must mean include — the behaviour that client was written against.
func TestAnUnsetExcludeIsAnInclude(t *testing.T) {
	got := scopeInputs([]*anubisv1.GrantScope{{Axis: "org", NodeId: "n1", Inherit: true}})
	if got[0].Exclude {
		t.Fatal("a scope that never mentions exclude must not become a carve-out")
	}
}

// scopeInputs is reached by both CreateGrant and SetMembershipEntries; a nil
// slice from either must not become a one-element slice of zeroes.
func TestNoScopesMapsToNoInputs(t *testing.T) {
	if got := scopeInputs(nil); len(got) != 0 {
		t.Fatalf("nil scopes produced %d inputs", len(got))
	}
}
