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
