package authzapp_test

import (
	"context"
	"errors"
	"testing"

	authzapp "github.com/gsoultan/anubis/internal/authz/app"
	authzdomain "github.com/gsoultan/anubis/internal/authz/domain"
	authzport "github.com/gsoultan/anubis/internal/authz/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
	"github.com/gsoultan/anubis/internal/shared/authctx"
)

// recordingRepo captures what tenant the interactor asked about — which is the
// only thing standing between a tenant-readable procedure and a cross-tenant
// read of any identity in the installation.
type recordingRepo struct {
	// Embedded so the fake satisfies the whole interface without stubbing
	// methods this use case never calls. A nil embed panics if one is reached,
	// which is the right outcome: it means the test is exercising something it
	// did not mean to.
	authzport.AuthzRepository
	sawTenant  string
	sawSubject string
	grants     []authzdomain.EffectiveGrant
	err        error
}

func (r *recordingRepo) EffectiveGrantsForIdentity(_ context.Context, tenantID, identityID string) ([]authzdomain.EffectiveGrant, error) {
	r.sawTenant, r.sawSubject = tenantID, identityID
	return r.grants, r.err
}

func withPrincipal(tenantID string) context.Context {
	return authctx.With(context.Background(), &authctx.Principal{
		IdentityID: "caller", TenantID: tenantID, TenantSlug: "impack",
	})
}

// TestTheTenantComesFromThePrincipalNotTheRequest is the load-bearing one.
//
// A subject id is not a secret and is frequently known across tenant
// boundaries. Taking the tenant from the caller's request would turn this into
// a read of any identity in the installation — the thing closing the admin
// plane was protecting, reopened through a convenience.
func TestTheTenantComesFromThePrincipalNotTheRequest(t *testing.T) {
	repo := &recordingRepo{}
	u := authzapp.NewEffectiveGrantsInteractor(repo)

	if _, err := u.Execute(withPrincipal("tenant-a"), "sub_1"); err != nil {
		t.Fatal(err)
	}
	if repo.sawTenant != "tenant-a" {
		t.Fatalf("queried tenant %q, want the principal's tenant-a", repo.sawTenant)
	}
	if repo.sawSubject != "sub_1" {
		t.Fatalf("queried subject %q", repo.sawSubject)
	}
}

// TestAnUnauthenticatedCallerIsRefused. There is no anonymous read here: the
// tenant is the principal's, so no principal means no tenant to scope to.
func TestAnUnauthenticatedCallerIsRefused(t *testing.T) {
	u := authzapp.NewEffectiveGrantsInteractor(&recordingRepo{})
	_, err := u.Execute(context.Background(), "sub_1")
	if !errors.Is(err, apperr.ErrUnauthenticated) {
		t.Fatalf("got %v, want ErrUnauthenticated", err)
	}
}

func TestAnEmptySubjectIsRefused(t *testing.T) {
	repo := &recordingRepo{}
	u := authzapp.NewEffectiveGrantsInteractor(repo)
	if _, err := u.Execute(withPrincipal("tenant-a"), ""); !errors.Is(err, apperr.ErrInvalidArgument) {
		t.Fatalf("got %v, want ErrInvalidArgument", err)
	}
	if repo.sawSubject != "" {
		t.Fatal("an empty subject reached the repository")
	}
}

// TestAnIdentityInAnotherTenantIsIndistinguishableFromOneWithNoGrants.
//
// Both are an empty list, deliberately. Answering "no such identity" would make
// this an existence oracle across tenants — a caller could enumerate who exists
// elsewhere in the installation without being able to read anything about them.
func TestAnIdentityInAnotherTenantLooksLikeOneWithNoGrants(t *testing.T) {
	u := authzapp.NewEffectiveGrantsInteractor(&recordingRepo{grants: nil})

	got, err := u.Execute(withPrincipal("tenant-a"), "somebody-elses-identity")
	if err != nil {
		t.Fatalf("a cross-tenant subject produced an error rather than an empty list: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d grants", len(got))
	}
}

// TestScopesAndExclusionsAreCarriedThrough. A caller that receives a grant
// without its carve-out builds one wider than Anubis holds.
func TestScopesAndExclusionsAreCarriedThrough(t *testing.T) {
	repo := &recordingRepo{grants: []authzdomain.EffectiveGrant{{
		ID: "g1", Role: "hr.reader", SelfScoped: true,
		Scopes: []authzdomain.GrantScope{
			{Axis: "org", NodeID: "jakarta", Inherit: true},
			{Axis: "org", NodeID: "surabaya", Exclude: true},
		},
	}}}

	got, err := authzapp.NewEffectiveGrantsInteractor(repo).Execute(withPrincipal("t"), "sub_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Scopes) != 2 {
		t.Fatalf("scopes were flattened: %+v", got)
	}
	if !got[0].HasExclusions() {
		t.Fatal("the carve-out was lost — the grant now reads wider than it is")
	}
	if !got[0].SelfScoped {
		t.Fatal("self_scoped was lost — the grant now reads as covering everybody's records")
	}
}

// TestARepositoryFailureIsInternalNotEmpty. An empty list means "no grants",
// and returning one on a database error would tell a PEP to deny everything
// while looking like a correct answer.
func TestARepositoryFailureIsInternalNotEmpty(t *testing.T) {
	repo := &recordingRepo{err: errors.New("connection refused")}
	_, err := authzapp.NewEffectiveGrantsInteractor(repo).Execute(withPrincipal("t"), "sub_1")
	if err == nil {
		t.Fatal("a database failure was reported as no grants")
	}
	// Compared on Code, not with errors.Is: apperr.Error.Wrap COPIES the
	// struct and the type implements no Is, so errors.Is(err, ErrInternal) is
	// false for every wrapped error in this codebase. Worth knowing before
	// writing an assertion that silently never fires.
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != apperr.ErrInternal.Code {
		t.Fatalf("got %v, want an internal apperr", err)
	}
}
