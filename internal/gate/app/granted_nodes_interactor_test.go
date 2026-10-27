package gateapp

import (
	"context"
	"testing"
	"time"

	authzreach "github.com/gsoultan/anubis/internal/authz/app/reach"
	"github.com/gsoultan/anubis/internal/gate/snapshot"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

type fixedSnapshots struct {
	data  *snapshot.Data
	fresh bool
}

func (f fixedSnapshots) Get(string) (*snapshot.Data, bool) { return f.data, f.fresh }

type grantedClock struct{}

func (grantedClock) Now() time.Time { return time.Unix(1_700_000_000, 0) }

func grantedSnapshot() *snapshot.Data {
	d := &snapshot.Data{
		LoadedAt:        time.Unix(1_699_999_990, 0),
		StrictAxes:      map[string]bool{},
		RolePermissions: map[string]map[string]bool{"approver": {"training:request:approve": true}},
		Permissions:     map[string]snapshot.Permission{"training:request:approve": {Key: "training:request:approve"}},
		Identities:      map[string]snapshot.Identity{"dewi": {TokenEpoch: 1}},
		RevokedSessions: map[string]bool{},
		Scope:           snapshot.NewScopeIndex([]snapshot.ScopeNode{{ID: "impack"}, {ID: "finance", Parent: "impack"}}),
		GrantsByIdentity: map[string][]snapshot.Grant{"dewi": {{ID: "g1", RoleID: "approver",
			Scopes: map[string][]snapshot.ScopeConstraint{"org": {{NodeID: "finance", Inherit: true}}}}}},
	}
	d.InternGrantScopes()
	return d
}

func tenantCtx() context.Context {
	return authctx.With(context.Background(), &authctx.Principal{TenantID: "t1", TenantSlug: "acme", Service: true})
}

func TestTheListIsServedFromAFreshSnapshotOnly(t *testing.T) {
	in := authzreach.NodesInput{Subject: "dewi", Permission: "training:request:approve", Axis: "org"}

	u := NewGrantedNodesInteractor(fixedSnapshots{data: grantedSnapshot(), fresh: true}, grantedClock{})
	out, err := u.Execute(tenantCtx(), in)
	if err != nil || len(out.Nodes) != 1 || out.Nodes[0].NodeID != "finance" || !out.Nodes[0].Inherit {
		t.Fatalf("fresh: %+v %v", out, err)
	}
	if out.AsOf.Unix() != 1_699_999_990 {
		t.Fatalf("as_of should be the snapshot's load time, got %v", out.AsOf)
	}

	// Stale is refused, not served: a list from an old snapshot can name a
	// place a revoked grant used to reach.
	stale := NewGrantedNodesInteractor(fixedSnapshots{data: grantedSnapshot(), fresh: false}, grantedClock{})
	if _, err := stale.Execute(tenantCtx(), in); apperr.AsError(err).Code != "snapshot_unavailable" {
		t.Fatalf("stale: want snapshot_unavailable, got %v", err)
	}
	none := NewGrantedNodesInteractor(fixedSnapshots{}, grantedClock{})
	if _, err := none.Execute(tenantCtx(), in); apperr.AsError(err).Code != "snapshot_unavailable" {
		t.Fatalf("missing: want snapshot_unavailable, got %v", err)
	}
	if ok, err := stale.CanReach(context.Background(), "acme", "dewi", "org", "finance"); ok || apperr.AsError(err).Code != "snapshot_unavailable" {
		t.Fatalf("CanReach on a stale snapshot must fail closed: ok=%v err=%v", ok, err)
	}
}

func TestTheListRefusesWhatItCannotAnswer(t *testing.T) {
	u := NewGrantedNodesInteractor(fixedSnapshots{data: grantedSnapshot(), fresh: true}, grantedClock{})
	good := authzreach.NodesInput{Subject: "dewi", Permission: "training:request:approve", Axis: "org"}

	if _, err := u.Execute(context.Background(), good); apperr.AsError(err).Code != "unauthenticated" {
		t.Fatalf("no principal: %v", err)
	}
	for name, in := range map[string]authzreach.NodesInput{
		"no subject":    {Permission: good.Permission, Axis: "org"},
		"no permission": {Subject: "dewi", Axis: "org"},
		"bad axis":      {Subject: "dewi", Permission: good.Permission, Axis: "not an axis!"},
		"bad fixed axis": {Subject: "dewi", Permission: good.Permission, Axis: "org",
			Scopes: map[string]string{"bad axis": "x"}},
	} {
		if _, err := u.Execute(tenantCtx(), in); apperr.AsError(err).Code != "invalid_argument" {
			t.Errorf("%s: want invalid_argument, got %v", name, err)
		}
	}
	// Unknown is not nowhere: a wiring mistake must be visible as one.
	unknown := authzreach.NodesInput{Subject: "stranger", Permission: good.Permission, Axis: "org"}
	if _, err := u.Execute(tenantCtx(), unknown); apperr.AsError(err).Code != "not_found" {
		t.Fatalf("unknown subject: want not_found, got %v", err)
	}
}
