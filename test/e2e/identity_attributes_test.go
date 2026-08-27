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
