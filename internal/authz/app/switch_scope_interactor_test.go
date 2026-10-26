package authzapp

import (
	"context"
	"errors"
	"testing"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	authapp "github.com/gsoultan/anubis/internal/auth/app"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	authport "github.com/gsoultan/anubis/internal/auth/port"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	scopeport "github.com/gsoultan/anubis/internal/scope/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// Stubs embed their port so a method this test does not exercise panics
// naming itself rather than returning a zero value that looks like a result.

type switchSessions struct {
	authport.SessionRepository
	pinned [][]byte
}

func (s *switchSessions) UpdateSessionScopes(_ context.Context, _ string, scopes []byte) error {
	s.pinned = append(s.pinned, scopes)
	return nil
}

func (s *switchSessions) SessionLive(_ context.Context, id string) (*authdomain.SessionView, error) {
	return &authdomain.SessionView{ID: id, IdentityID: "dewi", TenantID: "t1"}, nil
}

type switchNodes struct{ scopeport.ScopeNodeRepository }

func (switchNodes) ScopeNode(_ context.Context, _, id string) (*scopedomain.ScopeNodeRecord, error) {
	switch id {
	case "finance", "hr":
		return &scopedomain.ScopeNodeRecord{ID: id, Axis: "org", Status: "active"}, nil
	}
	return nil, errors.New("no such node")
}

type switchIssuer struct{ issued int }

func (i *switchIssuer) Issue(context.Context, authapp.IssueInput) (*authapp.TokenPair, error) {
	i.issued++
	return &authapp.TokenPair{AccessToken: "v4.public.switched"}, nil
}

type switchTx struct{}

func (switchTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type switchAudit struct{ events []auditdomain.AuditEvent }

func (a *switchAudit) Emit(_ context.Context, ev auditdomain.AuditEvent) {
	a.events = append(a.events, ev)
}

// fixedReach answers from a table: the places this subject holds a grant at.
type fixedReach struct {
	reach map[string]bool
	err   error
}

func (r fixedReach) CanReach(_ context.Context, _, _, axis, nodeID string) (bool, error) {
	return r.reach[axis+"/"+nodeID], r.err
}

func switchProbe(reach fixedReach) (SwitchScopeUsecase, *switchSessions, *switchIssuer, *switchAudit) {
	sessions, issuer, audit := &switchSessions{}, &switchIssuer{}, &switchAudit{}
	uc := NewSwitchScopeInteractor(sessions, switchNodes{}, nil, issuer, switchTx{}, audit, reach)
	return uc, sessions, issuer, audit
}

func dewiCtx() context.Context {
	return authctx.With(context.Background(), &authctx.Principal{
		IdentityID: "dewi", TenantID: "t1", TenantSlug: "acme", SessionID: "sess-1",
	})
}

// The bug this pins: a node that EXISTS is not a node the caller may stand
// at. The SDK documents that a refused switch means "not yours", and before
// this check any active node on the right axis was accepted.
func TestAPlaceTheCallerHoldsNoGrantAtIsRefused(t *testing.T) {
	uc, sessions, issuer, audit := switchProbe(fixedReach{reach: map[string]bool{"org/finance": true}})

	_, err := uc.Execute(dewiCtx(), map[string]string{"org": "hr"})
	if apperr.AsError(err).Code != "permission_denied" {
		t.Fatalf("an unheld place was accepted: %v", err)
	}
	if len(sessions.pinned) != 0 || issuer.issued != 0 {
		t.Fatal("the session was pinned or a token issued for a refused switch")
	}
	var denied bool
	for _, ev := range audit.events {
		if ev.Action == "auth.scope_switch" && ev.Result == "deny" {
			denied = true
		}
	}
	if !denied {
		t.Fatal("the refusal was not audited")
	}

	// And a place the caller does hold still switches.
	pair, err := uc.Execute(dewiCtx(), map[string]string{"org": "finance"})
	if err != nil || pair == nil || pair.AccessToken == "" {
		t.Fatalf("a held place was refused: %v", err)
	}
	if len(sessions.pinned) != 1 || issuer.issued != 1 {
		t.Fatalf("held place: pinned %d issued %d", len(sessions.pinned), issuer.issued)
	}
}

// The entitlement question is answered by the gate snapshot; when that is
// unavailable the switch fails closed rather than falling back to "exists".
func TestAnUnavailableSnapshotRefusesTheSwitch(t *testing.T) {
	uc, sessions, _, _ := switchProbe(fixedReach{err: apperr.ErrSnapshotUnavailable})
	_, err := uc.Execute(dewiCtx(), map[string]string{"org": "finance"})
	if !errors.Is(err, apperr.ErrSnapshotUnavailable) {
		t.Fatalf("want snapshot_unavailable, got %v", err)
	}
	if len(sessions.pinned) != 0 {
		t.Fatal("the session was pinned without an entitlement answer")
	}
}

// Existence is still checked first: a node that is not there is a bad
// request, not a permission question.
func TestANodeThatDoesNotExistIsStillABadRequest(t *testing.T) {
	uc, _, _, _ := switchProbe(fixedReach{reach: map[string]bool{"org/ghost": true}})
	_, err := uc.Execute(dewiCtx(), map[string]string{"org": "ghost"})
	if apperr.AsError(err).Code != "invalid_argument" {
		t.Fatalf("want invalid_argument, got %v", err)
	}
}
