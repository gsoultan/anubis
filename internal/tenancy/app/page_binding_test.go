package tenancyapp

import (
	"context"
	"strings"
	"testing"

	tenancydomain "github.com/gsoultan/anubis/internal/tenancy/domain"
	tenancyport "github.com/gsoultan/anubis/internal/tenancy/port"
)

// A page answers for an application OR a population, never both. Resolution
// (slug -> application -> realm -> default) would have to pick one, and
// whichever it picked would surprise somebody.
//
// auth_pages_one_binding refuses the row, so the data cannot go wrong either
// way. But a CHECK violation names a constraint, and the console needs to
// point at the input the operator got wrong — the console is where both
// bindings can now be chosen, so this is the layer that has to say which.
type stubApps struct {
	tenancyport.ApplicationRepository
	known bool
}

func (s *stubApps) ApplicationByID(_ context.Context, _, id string) (*tenancydomain.ApplicationRecord, error) {
	if !s.known {
		return nil, context.Canceled
	}
	return &tenancydomain.ApplicationRecord{ID: id}, nil
}

func normaliseWith(t *testing.T, in *tenancydomain.AuthPageInput) error {
	t.Helper()
	u := &pageAdminInteractor{apps: &stubApps{known: true}}
	_, err := u.normalise(context.Background(), "tenant-1", in)
	return err
}

func TestAPageCannotBindToBothAnApplicationAndARealm(t *testing.T) {
	err := normaliseWith(t, &tenancydomain.AuthPageInput{
		Kind: "signin", Slug: "both", Name: "Both",
		ApplicationID: "app-1", RealmID: "realm-1",
	})
	if err == nil {
		t.Fatal("a page bound to an application AND a realm was accepted")
	}
	// The message has to name a field, not a constraint: "auth_pages_one_binding
	// violated" tells an operator nothing about which input to change.
	if !strings.Contains(err.Error(), "realm") {
		t.Errorf("error does not name the offending field: %v", err)
	}
}

func TestEitherBindingAloneIsFine(t *testing.T) {
	for name, in := range map[string]*tenancydomain.AuthPageInput{
		"application": {Kind: "signin", Slug: "app-page", Name: "A", ApplicationID: "app-1"},
		"realm":       {Kind: "signin", Slug: "realm-page", Name: "R", RealmID: "realm-1"},
		"neither":     {Kind: "signin", Slug: "link-only", Name: "N"},
	} {
		if err := normaliseWith(t, in); err != nil {
			t.Errorf("%s binding rejected: %v", name, err)
		}
	}
}

// The slug becomes a published URL, so it is checked before anything is
// stored rather than left to the database.
func TestTheSlugIsValidatedBeforeStoring(t *testing.T) {
	// The rule is ^[a-z0-9][a-z0-9_-]{1,62}$ — shared with tenants and axes.
	// A single character is too short and a trailing hyphen is legal; neither
	// is obvious, which is why they are written down here.
	for _, bad := range []string{"", "x", "Has Spaces", "UPPER", "sl/ash", "-leading"} {
		err := normaliseWith(t, &tenancydomain.AuthPageInput{
			Kind: "signin", Slug: bad, Name: "N",
		})
		if err == nil {
			t.Errorf("slug %q was accepted", bad)
		}
	}
}

/*
UpdateApplication writes name, status, the redirect URI lists, the

	backchannel URI, token format and the TTLs. It does NOT write slug or kind,
	so a caller that changed either used to get 200 OK and no change — the same
	failure realms had, where an operator was told a correction had worked and
	it had not.

	slug IS the client_id, so changing it would break every configured client,
	and kind decides which auth model a client may use. Both being fixed is
	right; being fixed SILENTLY is not.
*/
func TestAnApplicationSlugAndKindAreRefusedNotIgnored(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		mutate      func(*tenancydomain.ApplicationRecord)
	}{
		{"slug", "slug", func(a *tenancydomain.ApplicationRecord) { a.Slug = "renamed" }},
		{"kind", "kind", func(a *tenancydomain.ApplicationRecord) { a.Kind = "native" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cur := tenancydomain.ApplicationRecord{ID: "app-1", Slug: "console", Kind: "spa"}
			in := cur
			tc.mutate(&in)
			err := checkAppIdentity(in, cur)
			if err == nil {
				t.Fatalf("a changed %s was accepted, which is the silent no-op", tc.name)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Errorf("error does not name %s: %v", tc.field, err)
			}
		})
	}
}

func TestAnUnchangedApplicationIdentityPasses(t *testing.T) {
	cur := tenancydomain.ApplicationRecord{ID: "app-1", Slug: "console", Kind: "spa"}
	if err := checkAppIdentity(cur, cur); err != nil {
		t.Errorf("an unchanged slug and kind were refused: %v", err)
	}
}
