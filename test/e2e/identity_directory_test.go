//go:build integration

package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	_ "github.com/jackc/pgx/v5/stdlib"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	"github.com/gsoultan/anubis/gen/go/anubis/v1/anubisv1connect"
)

// ADR-0013 says `identities.attributes` is encrypted at rest. This is the test
// that makes that sentence cost something.
//
// It writes real PII through the real API, then opens the database directly —
// the way anyone reading a stolen dump would — and demands that neither the
// values nor the field names appear there. Every earlier version of this
// promise passed by having nothing in the column at all.
func TestIdentityAttributesAreSealedInTheDatabase(t *testing.T) {
	requireServer(t)
	dbURL := os.Getenv("ANUBIS_DB_URL")
	if dbURL == "" {
		t.Skip("ANUBIS_DB_URL not set")
	}
	ctx := context.Background()
	token := platformLogin(t)
	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)

	username := fmt.Sprintf("pii-probe-%d", time.Now().UnixNano())
	created, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: "internal", Username: username, Password: "pii-probe-password-1234",
	}), token))
	if err != nil {
		t.Fatalf("create probe identity: %v", err)
	}
	id := created.Msg.GetIdentity().GetId()

	// Deliberately the kind of thing ADR-0013 is about: the field name is as
	// disclosing as the value.
	const (
		secretField = "diagnosis_code"
		secretValue = "F32.1"
	)
	attrs := map[string]string{secretField: secretValue, "home_address": "14 Rue Cler, Paris"}
	if _, err := idAdmin.SetIdentityAttributes(ctx, operatorBearer(connect.NewRequest(&anubisv1.SetIdentityAttributesRequest{
		Id: id, Attributes: attrs,
	}), token)); err != nil {
		t.Fatalf("set attributes: %v", err)
	}

	// 1. The API returns what was written.
	got, err := idAdmin.GetIdentityAttributes(ctx, operatorBearer(connect.NewRequest(&anubisv1.GetIdentityAttributesRequest{
		Id: id,
	}), token))
	if err != nil {
		t.Fatalf("get attributes: %v", err)
	}
	if got.Msg.Erased {
		t.Fatal("freshly written attributes reported as erased")
	}
	for k, want := range attrs {
		if got.Msg.Attributes[k] != want {
			t.Fatalf("%s: want %q, got %q", k, want, got.Msg.Attributes[k])
		}
	}

	// 2. The database holds none of it in the clear.
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	var stored, keyID string
	if err := db.QueryRowContext(ctx,
		`SELECT attributes::text, coalesce(pii_key_id::text,'') FROM identities WHERE id = $1`,
		id).Scan(&stored, &keyID); err != nil {
		t.Fatalf("read column: %v", err)
	}
	for _, secret := range []string{secretField, secretValue, "home_address", "Rue Cler"} {
		if strings.Contains(stored, secret) {
			t.Fatalf("%q is readable in identities.attributes: %s", secret, stored)
		}
	}
	if !strings.Contains(stored, `"sealed"`) {
		t.Fatalf("column does not hold a sealed envelope: %s", stored)
	}
	if keyID == "" {
		t.Fatal("attributes were written without minting a key for the identity")
	}

	// 3. Nor is the key itself in the clear: it is stored sealed under the
	//    master key, so a dump of pii_keys is no use on its own.
	var keyEnc []byte
	if err := db.QueryRowContext(ctx,
		`SELECT key_enc FROM pii_keys WHERE id = $1`, keyID).Scan(&keyEnc); err != nil {
		t.Fatalf("read key: %v", err)
	}
	if len(keyEnc) == 0 {
		t.Fatal("pii_keys row holds no material")
	}

	// 4. Plaintext cannot be put in the column even by someone with SQL
	//    access — the constraint from 0035 is the last line of defence.
	if _, err := db.ExecContext(ctx,
		`UPDATE identities SET attributes = '{"employee_id":"E-1"}'::jsonb WHERE id = $1`,
		id); err == nil {
		t.Fatal("the database accepted plaintext into identities.attributes")
	}

	// 5. Erasure is real: destroy the key and the ciphertext is noise. The
	//    identity row survives, so grants and audit entries still resolve.
	//
	//    The request goes through the API; the shred itself is the statement
	//    the retention sweep runs, because that sweep is a scheduled job with
	//    no RPC to trigger it. Same function, same reason code.
	if _, err := idAdmin.RequestErasure(ctx, operatorBearer(connect.NewRequest(&anubisv1.RequestErasureRequest{
		Id: id,
	}), token)); err != nil {
		t.Fatalf("request erasure: %v", err)
	}
	if _, err := db.ExecContext(ctx, `SELECT pii_shred($1, 'erasure_request')`, keyID); err != nil {
		t.Fatalf("shred: %v", err)
	}
	after, err := idAdmin.GetIdentityAttributes(ctx, operatorBearer(connect.NewRequest(&anubisv1.GetIdentityAttributesRequest{
		Id: id,
	}), token))
	if err != nil {
		t.Fatalf("get after shred: %v", err)
	}
	if !after.Msg.Erased {
		t.Fatalf("shredded identity did not report erasure: %+v", after.Msg)
	}
	if len(after.Msg.Attributes) != 0 {
		t.Fatalf("shredded attributes came back: %v", after.Msg.Attributes)
	}

	// 6. And writing again does not quietly resurrect it under a new key.
	_, err = idAdmin.SetIdentityAttributes(ctx, operatorBearer(connect.NewRequest(&anubisv1.SetIdentityAttributesRequest{
		Id: id, Attributes: map[string]string{"note": "re-added"},
	}), token))
	if err == nil {
		t.Fatal("an erased identity accepted new attributes, undoing the erasure")
	}
	if connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("want a conflict for a re-write after erasure, got %v", err)
	}
}

// Clearing attributes must leave nothing behind — an empty map is a deletion
// request, not a no-op.
func TestClearingAttributesEmptiesTheColumn(t *testing.T) {
	requireServer(t)
	dbURL := os.Getenv("ANUBIS_DB_URL")
	if dbURL == "" {
		t.Skip("ANUBIS_DB_URL not set")
	}
	ctx := context.Background()
	token := platformLogin(t)
	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)

	username := fmt.Sprintf("pii-clear-%d", time.Now().UnixNano())
	created, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: "internal", Username: username, Password: "pii-clear-password-1234",
	}), token))
	if err != nil {
		t.Fatalf("create probe identity: %v", err)
	}
	id := created.Msg.GetIdentity().GetId()

	set := func(a map[string]string) {
		t.Helper()
		if _, err := idAdmin.SetIdentityAttributes(ctx, operatorBearer(connect.NewRequest(&anubisv1.SetIdentityAttributesRequest{
			Id: id, Attributes: a,
		}), token)); err != nil {
			t.Fatalf("set %v: %v", a, err)
		}
	}
	set(map[string]string{"note": "temporary"})
	set(map[string]string{})

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	var stored string
	if err := db.QueryRowContext(ctx,
		`SELECT attributes::text FROM identities WHERE id = $1`, id).Scan(&stored); err != nil {
		t.Fatalf("read column: %v", err)
	}
	if stored != "{}" {
		t.Fatalf("cleared attributes left %s behind", stored)
	}
}

// A realm's `kind` decides which roles its members may hold — migrations/0010
// enforces that on every grant — so a realm created as `internal` when the
// operator meant `partner` lets employee-only roles reach outsiders.
//
// That mistake used to be permanent AND invisible: `kind` is not a column
// UpdateRealm writes, so an attempt to correct it returned 200 OK and changed
// nothing, and there is no API to delete a realm. The operator was told it
// worked.
//
// The rule now: correctable while the realm is empty, refused once it has
// members. An empty realm has decided nothing, and that is when a typo is
// actually noticed; a populated one would have its members' access
// retroactively re-decided.
func TestARealmKindIsCorrectableOnlyWhileEmpty(t *testing.T) {
	requireServer(t)
	ctx := context.Background()
	token := platformLogin(t)
	admin := anubisv1connect.NewTenantAdminServiceClient(http.DefaultClient, baseURL)
	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)

	code := fmt.Sprintf("kindprobe%d", time.Now().UnixNano()%1e6)
	created, err := admin.CreateRealm(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateRealmRequest{
		Realm: &anubisv1.Realm{
			Code: code, Kind: "internal", DisplayName: "Typed as internal by mistake",
			MinAssurance: 1,
			// Deliberately password-only: this test is about kind, and a
			// required factor with no deadline would not change it anyway.
			AllowedFactors: []string{"password"}, RequiredFactors: []string{"password"},
			SessionTtl: "8 hours", AccessTokenTtl: "10 minutes", RefreshTokenTtl: "30 days",
		},
	}), token))
	if err != nil {
		t.Fatalf("create realm: %v", err)
	}
	realm := created.Msg.GetRealm()
	if realm.Kind != "internal" {
		t.Fatalf("realm created as %q, wanted the mistake to stick for the test", realm.Kind)
	}

	// 1. Empty realm: the correction takes, and the response proves it rather
	//    than echoing back what was sent.
	realm.Kind = "partner"
	fixed, err := admin.UpdateRealm(ctx, operatorBearer(connect.NewRequest(&anubisv1.UpdateRealmRequest{
		Realm: realm,
	}), token))
	if err != nil {
		t.Fatalf("correcting an empty realm was refused: %v", err)
	}
	if got := fixed.Msg.GetRealm().GetKind(); got != "partner" {
		t.Fatalf("correction reported success and left kind %q — "+
			"this is the silent no-op the rule exists to prevent", got)
	}

	// 2. Give it a member. Now the realm has decided something.
	if _, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(&anubisv1.CreateIdentityRequest{
		Realm: code, Username: fmt.Sprintf("member-%d", time.Now().UnixNano()),
		Password: "kind-probe-password-1234",
	}), token)); err != nil {
		t.Fatalf("create identity in realm: %v", err)
	}

	// 3. Populated realm: refused, and refused loudly. A 200 here would mean
	//    the operator believes a privilege boundary moved when it did not.
	realm = fixed.Msg.GetRealm()
	realm.Kind = "internal"
	_, err = admin.UpdateRealm(ctx, operatorBearer(connect.NewRequest(&anubisv1.UpdateRealmRequest{
		Realm: realm,
	}), token))
	if err == nil {
		t.Fatal("a populated realm accepted a kind change — either it moved a " +
			"privilege boundary under its members, or it lied about doing so")
	}
	if connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Fatalf("want a conflict, got %v", err)
	}
	if !strings.Contains(err.Error(), "kind") {
		t.Fatalf("the refusal does not say which field was refused: %v", err)
	}

	// 4. And everything else about the realm still updates normally.
	realm.Kind = "partner"
	realm.DisplayName = "Supplier contacts"
	renamed, err := admin.UpdateRealm(ctx, operatorBearer(connect.NewRequest(&anubisv1.UpdateRealmRequest{
		Realm: realm,
	}), token))
	if err != nil {
		t.Fatalf("an ordinary policy update was blocked by the kind rule: %v", err)
	}
	if renamed.Msg.GetRealm().GetDisplayName() != "Supplier contacts" {
		t.Fatal("the rename did not take")
	}
}

// The People screen spans populations, so it asks for every category the
// tenant has defined and sends no realm id. `realm_id` is a uuid column, so
// the empty string was not an empty filter — it was `invalid input syntax for
// type uuid: ""`, and the whole call 500'd on every visit to the screen. The
// category label under each name had therefore never once rendered.
func TestRealmCategoriesListWithoutARealm(t *testing.T) {
	requireServer(t)
	dbURL := os.Getenv("ANUBIS_DB_URL")
	if dbURL == "" {
		t.Skip("ANUBIS_DB_URL not set")
	}
	ctx := context.Background()
	token := platformLogin(t)
	tenantAdmin := anubisv1connect.NewTenantAdminServiceClient(http.DefaultClient, baseURL)

	realms, err := tenantAdmin.ListRealms(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.ListRealmsRequest{}), token))
	if err != nil {
		t.Fatalf("list realms: %v", err)
	}
	if len(realms.Msg.GetRealms()) == 0 {
		t.Fatal("the tenant has no populations at all")
	}
	known := map[string]bool{}
	for _, r := range realms.Msg.GetRealms() {
		known[r.GetId()] = true
	}
	/* Prefer a population that already has a category somebody is IN, so the
	   second half of this test -- that naming a realm does count -- actually
	   runs. realms[0] is whatever sorts first, which on a database carrying
	   leftover probe realms is an empty one. */
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	home := realms.Msg.GetRealms()[0]
	var populatedRealm string
	if err := db.QueryRowContext(ctx,
		`SELECT c.realm_id::text FROM realm_categories c
		   JOIN identities i ON i.category_id = c.id
		  GROUP BY c.realm_id LIMIT 1`).Scan(&populatedRealm); err == nil {
		for _, r := range realms.Msg.GetRealms() {
			if r.GetId() == populatedRealm {
				home = r
			}
		}
	}

	/* The fixture makes its own category rather than trusting the dataset.
	   Migration 0012 seeds one only for realms that exist WHEN IT RUNS, and a
	   database built migrations-first has none — which is how an assertion
	   that "the seeded tenant defines several" passed here and failed on CI. */
	code := fmt.Sprintf("catprobe%d", time.Now().UnixNano())
	made, err := tenantAdmin.CreateRealmCategory(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.CreateRealmCategoryRequest{Category: &anubisv1.RealmCategory{
			RealmId: home.GetId(), Code: code, DisplayName: "Category probe", SortOrder: 900,
		}}), token))
	if err != nil {
		t.Fatalf("create probe category: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(),
			`DELETE FROM realm_categories WHERE id = $1`, made.Msg.GetCategory().GetId()); err != nil {
			t.Errorf("probe category not removed: %v", err)
		}
	})

	all, err := tenantAdmin.ListRealmCategories(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.ListRealmCategoriesRequest{RealmId: ""}), token))
	if err != nil {
		t.Fatalf("list categories across all populations: %v", err)
	}
	var found bool
	for _, c := range all.Msg.GetCategories() {
		if c.GetCode() == code {
			found = true
		}
		// A code is unique per realm, so a consumer resolving a name needs both.
		if c.GetRealmId() == "" {
			t.Fatalf("category %q carries no realm id", c.GetCode())
		}
		if !known[c.GetRealmId()] {
			t.Fatalf("category %q belongs to realm %s, which is not this tenant's",
				c.GetCode(), c.GetRealmId())
		}
	}
	if !found {
		t.Fatalf("the tenant-wide listing did not contain %q, which was just "+
			"created in %s — the filter matched nothing rather than everything",
			code, home.GetCode())
	}

	// Narrowing to one realm must return a subset of the same answer, or the
	// two code paths disagree about what a category is.
	one, err := tenantAdmin.ListRealmCategories(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.ListRealmCategoriesRequest{RealmId: home.GetId()}), token))
	if err != nil {
		t.Fatalf("list categories for one population: %v", err)
	}
	var narrowed bool
	for _, c := range one.Msg.GetCategories() {
		if c.GetRealmId() != home.GetId() {
			t.Fatalf("asked for realm %s, got a category from %s",
				home.GetId(), c.GetRealmId())
		}
		if c.GetCode() == code {
			narrowed = true
		}
	}
	if !narrowed {
		t.Fatalf("realm %s reported no %q, but it was created there", home.GetCode(), code)
	}

	/* identity_count is the population's figure, and computing it tenant-wide
	   is a grouped scan of the entire directory — 338ms over 14k buffers on a
	   57k-identity tenant, for a number the screen making that call never
	   displays. Naming a realm is what asks for it. */
	for _, c := range all.Msg.GetCategories() {
		if c.GetIdentityCount() != 0 {
			t.Fatalf("category %q carries a count on the tenant-wide listing (%d): "+
				"counting every realm is a scan of the whole directory, and no "+
				"caller of this shape reads the number",
				c.GetCode(), c.GetIdentityCount())
		}
	}
	// The other half — that naming a realm DOES count — needs a category some
	// people are actually in, which a freshly migrated database need not have.
	var populated string
	if err := db.QueryRowContext(ctx,
		`SELECT c.code FROM realm_categories c
		   JOIN identities i ON i.category_id = c.id
		  WHERE c.realm_id = $1 GROUP BY c.code LIMIT 1`, home.GetId()).Scan(&populated); err != nil {
		t.Logf("no populated category in %s, so the counting half is unproven here", home.GetCode())
		return
	}
	for _, c := range one.Msg.GetCategories() {
		if c.GetCode() == populated && c.GetIdentityCount() == 0 {
			t.Fatalf("category %q has members in the database but the "+
				"realm-scoped listing reported none", populated)
		}
	}
}

// The guard proved WHICH tenant was asking and the query then ignored the
// answer: it keyed on realm_id alone. An operator who learned another
// tenant's realm id read that tenant's categories — the directory
// classification of people they have no relationship with.
func TestRealmCategoriesRefuseAnotherTenantsRealm(t *testing.T) {
	requireServer(t)
	dbURL := os.Getenv("ANUBIS_DB_URL")
	if dbURL == "" {
		t.Skip("ANUBIS_DB_URL not set")
	}
	ctx := context.Background()
	token := platformLogin(t)

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Registered BEFORE the row cleanup, and NOT deferred: t.Cleanup runs
	// after the function returns, so a `defer db.Close()` would shut the pool
	// before the delete below could use it — which is how the first run of
	// this test left its planted tenant in the dev database. Cleanup is LIFO,
	// so the delete goes last-registered and therefore runs first.
	t.Cleanup(func() { _ = db.Close() })

	// A realm and a category belonging to a tenant this operator does not
	// administer. Planted directly, because the API has no way to reach
	// another tenant — which is the whole point.
	suffix := time.Now().UnixNano()
	var otherTenant string
	if err := db.QueryRowContext(ctx,
		`INSERT INTO tenants (name, slug) VALUES ($1, $2) RETURNING id`,
		fmt.Sprintf("Neighbour %d", suffix),
		fmt.Sprintf("neighbour-%d", suffix),
	).Scan(&otherTenant); err != nil {
		t.Fatalf("plant tenant: %v", err)
	}
	t.Cleanup(func() {
		// Reported, not swallowed: a planted tenant left behind is a probe
		// realm that every later snapshot rebuild will load forever.
		if _, err := db.ExecContext(context.Background(),
			`DELETE FROM tenants WHERE id = $1`, otherTenant); err != nil {
			t.Errorf("planted tenant %s not removed: %v", otherTenant, err)
		}
	})

	var otherRealm string
	if err := db.QueryRowContext(ctx,
		`INSERT INTO realms (tenant_id, code, kind, display_name)
		 VALUES ($1, 'internal', 'internal', 'Neighbour staff') RETURNING id`,
		otherTenant,
	).Scan(&otherRealm); err != nil {
		t.Fatalf("plant realm: %v", err)
	}
	const secretCode = "board_member"
	if _, err := db.ExecContext(ctx,
		`INSERT INTO realm_categories (tenant_id, realm_id, code, display_name, sort_order)
		 VALUES ($1, $2, $3, 'Board member', 0)`,
		otherTenant, otherRealm, secretCode,
	); err != nil {
		t.Fatalf("plant category: %v", err)
	}

	tenantAdmin := anubisv1connect.NewTenantAdminServiceClient(http.DefaultClient, baseURL)
	resp, err := tenantAdmin.ListRealmCategories(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.ListRealmCategoriesRequest{RealmId: otherRealm}), token))
	// Empty or refused are both acceptable; handing the rows over is not.
	if err != nil {
		return
	}
	for _, c := range resp.Msg.GetCategories() {
		if c.GetCode() == secretCode || c.GetRealmId() == otherRealm {
			t.Fatalf("read another tenant's category %q by supplying their realm id",
				c.GetCode())
		}
	}
	if n := len(resp.Msg.GetCategories()); n != 0 {
		t.Fatalf("another tenant's realm returned %d categories", n)
	}
}

// `identities.retention_until` and its partial index have existed since
// migrations/0008, and the retention sweeper writes it — but no read path ever
// selected the column, so the proto could not carry it and the console's
// Retention column printed a dash for every person, including the ones with a
// real statutory deadline.
func TestIdentityCarriesItsRetentionDeadline(t *testing.T) {
	requireServer(t)
	dbURL := os.Getenv("ANUBIS_DB_URL")
	if dbURL == "" {
		t.Skip("ANUBIS_DB_URL not set")
	}
	ctx := context.Background()
	token := platformLogin(t)
	idAdmin := anubisv1connect.NewIdentityAdminServiceClient(http.DefaultClient, baseURL)

	username := fmt.Sprintf("retention-probe-%d", time.Now().UnixNano())
	created, err := idAdmin.CreateIdentity(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.CreateIdentityRequest{
			Realm: "internal", Username: username, Password: "retention-probe-password-1234",
		}), token))
	if err != nil {
		t.Fatalf("create probe identity: %v", err)
	}
	id := created.Msg.GetIdentity().GetId()

	// An internal realm has no statutory limit, so the probe starts with none
	// — which is exactly the value the broken column was indistinguishable
	// from.
	if got := created.Msg.GetIdentity().GetRetentionUntil(); got != 0 {
		t.Fatalf("fresh internal identity already carries a retention deadline: %d", got)
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	deadline := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	if _, err := db.ExecContext(ctx,
		`UPDATE identities SET retention_until = $1 WHERE id = $2`, deadline, id); err != nil {
		t.Fatalf("set retention deadline: %v", err)
	}

	one, err := idAdmin.GetIdentity(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.GetIdentityRequest{Id: id}), token))
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if got := one.Msg.GetIdentity().GetRetentionUntil(); got != deadline.Unix() {
		t.Fatalf("GetIdentity retention_until = %d, want %d", got, deadline.Unix())
	}

	// The list path selects its own columns, so it can be wrong separately.
	list, err := idAdmin.ListIdentities(ctx, operatorBearer(connect.NewRequest(
		&anubisv1.ListIdentitiesRequest{Realm: "internal", Query: username, PageSize: 10}), token))
	if err != nil {
		t.Fatalf("list identities: %v", err)
	}
	var seen bool
	for _, i := range list.Msg.GetIdentities() {
		if i.GetId() != id {
			continue
		}
		seen = true
		if got := i.GetRetentionUntil(); got != deadline.Unix() {
			t.Fatalf("ListIdentities retention_until = %d, want %d", got, deadline.Unix())
		}
	}
	if !seen {
		t.Fatalf("probe identity %s absent from its own realm listing", username)
	}
}
