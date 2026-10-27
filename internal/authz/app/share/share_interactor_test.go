package authzshare

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	"github.com/gsoultan/anubis/internal/authz/domain/share"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	authzshareport "github.com/gsoultan/anubis/internal/authz/port/share"
	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
	identityport "github.com/gsoultan/anubis/internal/identity/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// Stubs embed their port so a method this test does not exercise panics
// naming itself rather than returning a zero value that looks like a result.

// stubAuthz answers Authorize from a table of "identity|permission|targets".
type stubAuthz struct {
	authzport.AuthzRepository
	allow map[string]bool
	asked []string
}

func (s *stubAuthz) Authorize(_ context.Context, identityID, _, permission string, targets []byte) (bool, error) {
	var t map[string]string
	_ = json.Unmarshal(targets, &t)
	key := identityID + "|" + permission + "|" + t["org"]
	if _, named := t["_record"]; named {
		key += "|record"
	}
	s.asked = append(s.asked, key)
	return s.allow[key], nil
}

func (s *stubAuthz) PermissionByKey(_ context.Context, _, key string) (*authzdomain.PermissionMeta, error) {
	if strings.HasPrefix(key, "training:") {
		return &authzdomain.PermissionMeta{}, nil
	}
	return nil, errors.New("unknown")
}

type stubIdentities struct {
	identityport.IdentityRepository
}

func (stubIdentities) Identity(_ context.Context, _, id string) (*identitydomain.Identity, error) {
	switch id {
	case "rudi":
		return &identitydomain.Identity{ID: id, Status: "active"}, nil
	case "gone":
		return &identitydomain.Identity{ID: id, Status: "disabled", Disabled: true}, nil
	}
	return nil, errors.New("not found")
}

type stubShares struct {
	authzshareport.ShareRepository
	created []share.CreateInput
	stored  map[string]*share.Share
	revoked []string
}

func (s *stubShares) CreateShare(_ context.Context, in share.CreateInput) (*share.Share, error) {
	s.created = append(s.created, in)
	return &share.Share{ID: "sh-1", TenantID: in.TenantID, FromIdentityID: in.FromIdentityID,
		ToIdentityID: in.ToIdentityID, Record: in.Record, Permissions: in.Permissions,
		RecordScopes: in.RecordScopes, ValidUntil: in.ValidUntil}, nil
}

func (s *stubShares) Share(_ context.Context, _, id string) (*share.Share, error) {
	if sh, ok := s.stored[id]; ok {
		return sh, nil
	}
	return nil, errors.New("not found")
}

func (s *stubShares) SharesOnRecord(_ context.Context, _, record string) ([]share.Share, error) {
	var out []share.Share
	for _, sh := range s.stored {
		if sh.Record == record {
			out = append(out, *sh)
		}
	}
	return out, nil
}

func (s *stubShares) RevokeShare(_ context.Context, _, id, _ string) (bool, error) {
	s.revoked = append(s.revoked, id)
	return true, nil
}

type stubClock struct{}

func (stubClock) Now() time.Time { return time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC) }

type stubAudit struct{ events []auditdomain.AuditEvent }

func (a *stubAudit) Emit(_ context.Context, ev auditdomain.AuditEvent) {
	a.events = append(a.events, ev)
}

func asPerson(id string) context.Context {
	return authctx.With(context.Background(), &authctx.Principal{IdentityID: id, TenantID: "t1", SessionID: "s1"})
}

func probe(allow map[string]bool, stored ...*share.Share) (ShareUsecase, *stubShares, *stubAuthz, *stubAudit) {
	shares := &stubShares{stored: map[string]*share.Share{}}
	for _, sh := range stored {
		shares.stored[sh.ID] = sh
	}
	authz := &stubAuthz{allow: allow}
	audit := &stubAudit{}
	return NewShareInteractor(shares, authz, stubIdentities{}, stubClock{}, audit), shares, authz, audit
}

func goodInput() ShareInput {
	return ShareInput{
		RecordKind: "job_requisition", RecordRef: "JR-0142",
		RecordScopes: map[string]string{"org": "finance", "_record": "job_requisition/JR-0142"},
		ToIdentityID: "rudi", Permissions: []string{"training:request:view", "training:request:comment"},
		ValidUntil: stubClock{}.Now().Add(7 * 24 * time.Hour), Note: "please review",
	}
}

// The rule the system being replaced lacked: you can only share what you
// can do, and "can do" is decided by grants at the record's places — a share
// the sharer holds does not count, so "_record" is stripped from the probe.
func TestYouCanOnlyShareWhatYouCanDo(t *testing.T) {
	uc, shares, authz, audit := probe(map[string]bool{
		"dewi|training:request:view|finance":    true,
		"dewi|training:request:comment|finance": true,
	})
	sh, err := uc.Share(asPerson("dewi"), goodInput())
	if err != nil {
		t.Fatalf("a share of held permissions was refused: %v", err)
	}
	if sh.Record != "job_requisition/JR-0142" || len(shares.created) != 1 {
		t.Fatalf("share not stored as expected: %+v", sh)
	}
	for _, ask := range authz.asked {
		if strings.HasSuffix(ask, "|record") {
			t.Fatalf("the sharer was probed with _record, so a share could beget a share: %s", ask)
		}
	}
	if _, kept := shares.created[0].RecordScopes["_record"]; kept {
		t.Fatal("_record was stored among the record's places")
	}
	var audited bool
	for _, ev := range audit.events {
		if ev.Action == "share.create" && ev.Result == "allow" && ev.TargetID == "rudi" {
			audited = true
		}
	}
	if !audited {
		t.Fatal("the share was not audited")
	}

	// One permission the sharer lacks refuses the whole share.
	uc, shares, _, audit = probe(map[string]bool{"dewi|training:request:view|finance": true})
	_, err = uc.Share(asPerson("dewi"), goodInput())
	if apperr.AsError(err).Code != "permission_denied" {
		t.Fatalf("sharing an unheld permission: want permission_denied, got %v", err)
	}
	if len(shares.created) != 0 {
		t.Fatal("a refused share was stored")
	}
	if len(audit.events) == 0 || audit.events[0].Result != "deny" {
		t.Fatal("the refusal was not audited")
	}
}

func TestAShareIsBoundedBeforeAnyoneIsAsked(t *testing.T) {
	allow := map[string]bool{"dewi|training:request:view|finance": true, "dewi|training:request:comment|finance": true}
	cases := map[string]func(in *ShareInput){
		"to self":            func(in *ShareInput) { in.ToIdentityID = "dewi" },
		"no permissions":     func(in *ShareInput) { in.Permissions = nil },
		"unknown permission": func(in *ShareInput) { in.Permissions = []string{"nope:x:y"} },
		"never ends":         func(in *ShareInput) { in.ValidUntil = time.Time{} },
		"ends in the past":   func(in *ShareInput) { in.ValidUntil = stubClock{}.Now().Add(-time.Minute) },
		"a year":             func(in *ShareInput) { in.ValidUntil = stubClock{}.Now().Add(365 * 24 * time.Hour) },
		"bad kind":           func(in *ShareInput) { in.RecordKind = "Job Requisition" },
		"empty ref":          func(in *ShareInput) { in.RecordRef = "  " },
		"disabled recipient": func(in *ShareInput) { in.ToIdentityID = "gone" },
		"unknown recipient":  func(in *ShareInput) { in.ToIdentityID = "nobody" },
	}
	for name, mutate := range cases {
		uc, shares, _, _ := probe(allow)
		in := goodInput()
		mutate(&in)
		_, err := uc.Share(asPerson("dewi"), in)
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
		if len(shares.created) != 0 {
			t.Errorf("%s: stored", name)
		}
	}
	uc, _, _, _ := probe(allow)
	if _, err := uc.Share(authctx.With(context.Background(), &authctx.Principal{IdentityID: "op", Platform: true}), goodInput()); !errors.Is(err, apperr.ErrUnauthenticated) {
		t.Fatalf("an operator shared: %v", err)
	}
	if _, err := uc.Share(context.Background(), goodInput()); !errors.Is(err, apperr.ErrUnauthenticated) {
		t.Fatalf("nobody shared: %v", err)
	}
}

// Revoking: the sharer always may; another person only if they hold, by
// grant, everything the share gives at the record's own places.
func TestWhoMayEndAShare(t *testing.T) {
	existing := &share.Share{ID: "sh-1", TenantID: "t1", FromIdentityID: "dewi", ToIdentityID: "rudi",
		Record: "job_requisition/JR-0142", Permissions: []string{"training:request:view"},
		RecordScopes: map[string]string{"org": "finance"}, ValidUntil: stubClock{}.Now().Add(time.Hour)}

	uc, shares, _, _ := probe(nil, existing)
	if err := uc.Unshare(asPerson("dewi"), "sh-1", "done"); err != nil || len(shares.revoked) != 1 {
		t.Fatalf("the sharer could not end their own share: %v", err)
	}

	uc, shares, _, _ = probe(nil, existing)
	if err := uc.Unshare(asPerson("sari"), "sh-1", ""); apperr.AsError(err).Code != "permission_denied" || len(shares.revoked) != 0 {
		t.Fatalf("a bystander ended somebody's share: %v", err)
	}

	uc, shares, _, _ = probe(map[string]bool{"sari|training:request:view|finance": true}, existing)
	if err := uc.Unshare(asPerson("sari"), "sh-1", "no longer needed"); err != nil || len(shares.revoked) != 1 {
		t.Fatalf("somebody holding the access could not end the share: %v", err)
	}

	uc, _, _, _ = probe(nil)
	if err := uc.Unshare(asPerson("dewi"), "missing", ""); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("missing share: %v", err)
	}
}

// Listing follows access: the two parties see a share; a third person sees
// it only if they could have made it.
func TestWhoSeesASharedRecord(t *testing.T) {
	sh := &share.Share{ID: "sh-1", TenantID: "t1", FromIdentityID: "dewi", ToIdentityID: "rudi",
		Record: "job_requisition/JR-0142", Permissions: []string{"training:request:view"},
		RecordScopes: map[string]string{"org": "finance"}, ValidUntil: stubClock{}.Now().Add(time.Hour)}
	allow := map[string]bool{"sari|training:request:view|finance": true}
	for who, want := range map[string]int{"dewi": 1, "rudi": 1, "sari": 1, "nobody": 0} {
		uc, _, _, _ := probe(allow, sh)
		got, err := uc.SharesOnRecord(asPerson(who), "job_requisition", "JR-0142", map[string]string{"org": "finance"})
		if err != nil || len(got) != want {
			t.Errorf("%s sees %d (err %v), want %d", who, len(got), err, want)
		}
	}
}
