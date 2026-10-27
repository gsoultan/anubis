package authzapp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// stubEngine says yes to everything and remembers what it was asked, so
// the test can tell a refusal made HERE from one the engine made.
type stubEngine struct {
	authzport.AuthzRepository
	asked []string
}

func (s *stubEngine) Authorize(_ context.Context, identityID, _, permission string, targets []byte) (bool, error) {
	var t map[string]string
	_ = json.Unmarshal(targets, &t)
	s.asked = append(s.asked, identityID+"|"+permission+"|"+t["_record"])
	return true, nil
}

func (s *stubEngine) PermissionByKey(context.Context, string, string) (*authzdomain.PermissionMeta, error) {
	return &authzdomain.PermissionMeta{}, nil
}

type linkClock struct{}

func (linkClock) Now() time.Time { return time.Unix(1_700_000_000, 0) }

type linkAudit struct{ events []auditdomain.AuditEvent }

func (a *linkAudit) Emit(_ context.Context, ev auditdomain.AuditEvent) {
	a.events = append(a.events, ev)
}

func linkPrincipal(record string) context.Context {
	return authctx.With(context.Background(), &authctx.Principal{
		IdentityID: "rudi", TenantID: "t1", TenantSlug: "acme",
		AMR: []string{"link"}, Scopes: map[string]string{"_record": record},
	})
}

// A token minted from an access link decides exactly one thing: its own
// subject on its own record. Everything else is refused HERE, before the
// engine — which would otherwise answer from the recipient's full grants.
func TestALinkTokenDecidesOnlyItsOwnRecord(t *testing.T) {
	engine := &stubEngine{}
	uc := NewAuthorizeInteractor(engine, linkClock{}, &linkAudit{})
	const rec = "job_requisition/JR-0142"

	d, err := uc.Execute(linkPrincipal(rec), AuthorizeInput{
		Subject: "rudi", Permission: "ats:requisition:comment",
		Scopes: map[string]string{"org": "finance", "_record": rec},
	})
	if err != nil || !d.Allow {
		t.Fatalf("own record refused: %+v %v", d, err)
	}
	if len(engine.asked) != 1 {
		t.Fatalf("engine asked %d times", len(engine.asked))
	}

	for name, in := range map[string]AuthorizeInput{
		"another record":   {Subject: "rudi", Permission: "ats:requisition:comment", Scopes: map[string]string{"_record": "job_requisition/JR-0001"}},
		"no record":        {Subject: "rudi", Permission: "ats:requisition:comment", Scopes: map[string]string{"org": "finance"}},
		"another subject":  {Subject: "dewi", Permission: "ats:requisition:comment", Scopes: map[string]string{"_record": rec}},
		"a place, no more": {Subject: "rudi", Permission: "ats:requisition:list", Scopes: nil},
	} {
		d, err := uc.Execute(linkPrincipal(rec), in)
		if err != nil || d.Allow || d.Reason != "link_bound" {
			t.Errorf("%s: want link_bound denial, got %+v %v", name, d, err)
		}
	}
	if len(engine.asked) != 1 {
		t.Fatalf("a refused link question reached the engine: %v", engine.asked)
	}
}

// An ordinary principal asking with a record named is not link-bound: the
// engine decides, as before.
func TestAnOrdinaryPrincipalIsNotLinkBound(t *testing.T) {
	engine := &stubEngine{}
	uc := NewAuthorizeInteractor(engine, linkClock{}, &linkAudit{})
	ctx := authctx.With(context.Background(), &authctx.Principal{IdentityID: "svc", TenantID: "t1", Service: true, AMR: []string{"client_secret"}})
	d, err := uc.Execute(ctx, AuthorizeInput{Subject: "rudi", Permission: "ats:requisition:comment",
		Scopes: map[string]string{"_record": "job_requisition/JR-0142"}})
	if err != nil || !d.Allow || len(engine.asked) != 1 {
		t.Fatalf("ordinary ask not passed to the engine: %+v %v asked=%v", d, err, engine.asked)
	}
}
