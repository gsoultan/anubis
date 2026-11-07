package authzapp

import (
	"context"
	"testing"

	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// A person's own sign-in asks about that person, and only that person.
//
// Root cause: Authorize and Explain took the subject from the request and the
// tenant from the token, and checked nothing between them. Any signed-in
// person, a self-registered public user included, could ask about anybody in
// the tenant, and Explain answered with that person's roles and grant ids.
func TestAPersonsTokenAsksOnlyAboutThatPerson(t *testing.T) {
	ctx := authctx.With(context.Background(), &authctx.Principal{
		IdentityID: "rudi", TenantID: "t1", TenantSlug: "acme", SessionID: "s1", AMR: []string{"pwd"},
	})
	engine := &stubEngine{}
	authorize := NewAuthorizeInteractor(engine, linkClock{}, &linkAudit{})
	explain := NewExplainInteractor(engine)

	if d, err := authorize.Execute(ctx, AuthorizeInput{Subject: "rudi", Permission: "ats:requisition:read"}); err != nil || !d.Allow {
		t.Fatalf("a person asking about themselves was refused: %+v %v", d, err)
	}
	if d, err := authorize.Execute(ctx, AuthorizeInput{Subject: "dewi", Permission: "ats:requisition:read"}); err == nil && d.Allow {
		t.Fatalf("a person's token was answered about somebody else: %+v", d)
	}
	if _, err := explain.Execute(ctx, AuthorizeInput{Subject: "dewi", Permission: "ats:requisition:read"}); err == nil {
		t.Fatal("Explain narrated somebody else's grants to a person's token")
	}
	if len(engine.asked) != 1 {
		t.Fatalf("the engine was asked %d times; only the person's own question should reach it", len(engine.asked))
	}

	// The tenant's system asks about anyone: that is what the API is for.
	svc := authctx.With(context.Background(), &authctx.Principal{IdentityID: "key-1", TenantID: "t1", Service: true})
	if d, err := authorize.Execute(svc, AuthorizeInput{Subject: "dewi", Permission: "ats:requisition:read"}); err != nil || !d.Allow {
		t.Fatalf("an API key could not ask about a subject: %+v %v", d, err)
	}
}

func (s *stubEngine) AuthorizeExplain(_ context.Context, identityID, _, permission string, _ []byte) (string, error) {
	s.asked = append(s.asked, identityID+"|"+permission+"|explain")
	return `{"allow": true}`, nil
}
