//go:build integration

package stepup

import (
	"context"
	"testing"
	"time"

	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	authzpg "github.com/gsoultan/anubis/internal/authz/adapter/postgres"
	authzapp "github.com/gsoultan/anubis/internal/authz/app"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

type dropAudit struct{}

func (dropAudit) Emit(context.Context, auditdomain.AuditEvent) {}

// A permission that demands a sign-in from the last day must refuse one from
// two days ago.
//
// Root cause: the online decision read max_auth_age as interval TEXT and
// parsed it in Go, and Postgres renders a whole number of days with no clock
// part ("1 day", "7 days", "1 mon"), which the parser did not accept. The
// failure was swallowed as "no recency requirement", so every whole-day
// max_auth_age was silently not enforced on the Authorize RPC while the gate,
// which reads seconds, enforced it.
func TestWholeDayMaxAuthAgeIsEnforced(t *testing.T) {
	if pool == nil {
		t.Skip("ANUBIS_DB_URL not set")
	}
	now := time.Now()
	uc := authzapp.NewAuthorizeInteractor(authzpg.New(database.New(pool)), fixedClock{now}, dropAudit{})
	ctx := authctx.With(context.Background(), &authctx.Principal{
		IdentityID: w.identity, TenantID: w.tenant, Service: true,
	})

	fresh, err := uc.Execute(ctx, authzapp.AuthorizeInput{
		Subject: w.identity, Permission: w.perm,
		AMR: []string{"pwd"}, AuthTime: now.Add(-time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fresh.Allow {
		t.Fatalf("a sign-in from an hour ago was refused: %+v", fresh)
	}

	stale, err := uc.Execute(ctx, authzapp.AuthorizeInput{
		Subject: w.identity, Permission: w.perm,
		AMR: []string{"pwd"}, AuthTime: now.Add(-48 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if stale.Allow || stale.Reason != "step_up_required" {
		t.Fatalf("max_auth_age '1 day' let a two-day-old sign-in through: %+v", stale)
	}
	if stale.MaxAuthAge != (24 * time.Hour).String() {
		t.Errorf("max auth age reported as %q, want 24h0m0s", stale.MaxAuthAge)
	}
}
