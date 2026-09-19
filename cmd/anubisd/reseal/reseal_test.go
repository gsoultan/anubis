package reseal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	authpg "github.com/gsoultan/anubis/internal/auth/adapter/postgres"
	authdomain "github.com/gsoultan/anubis/internal/auth/domain"
	controlpg "github.com/gsoultan/anubis/internal/control/adapter/postgres"
	identitypg "github.com/gsoultan/anubis/internal/identity/adapter/postgres"
	"github.com/gsoultan/anubis/internal/platform/crypto/keyring"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Rewrapping under the wrong additional authenticated data produces
// ciphertext that decrypts to nothing, silently, for every secret in the
// installation. There is no recovering from it and no alarm when it happens —
// the rows look fine until somebody tries to sign in.
//
// So this test does not assert that reseal "ran". It seals each of the three
// shapes through the PRODUCTION write path, rewraps, and then reads each one
// back through the PRODUCTION read path under the new master. If an AAD here
// disagrees with the one the rest of the system uses, the read fails.
func TestResealKeepsEverySecretReadable(t *testing.T) {
	dsn := os.Getenv("ANUBIS_DB_URL")
	if dsn == "" {
		t.Skip("ANUBIS_DB_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	db := database.New(pool)

	oldMaster := randomKey(t)
	newMaster := randomKey(t)

	keyStore := authpg.New(db)
	idStore := identitypg.New(db)
	ctlStore := controlpg.New(db)

	n := time.Now().UnixNano()

	// ---- a signing key, sealed with AAD = kid -----------------------------
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	kid := fmt.Sprintf("zzreseal-%d", n)
	sealedKey, err := keyring.SealSecret(oldMaster, kid, seed)
	if err != nil {
		t.Fatal(err)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	if err := keyStore.CreateKey(ctx, authdomain.KeyRecord{
		Kid: kid, Alg: "Ed25519", Status: keyring.StatusRetiring,
		Purpose: keyring.PurposeAccess, PublicKey: priv.Public().(ed25519.PublicKey),
		PrivateKeyEnc: sealedKey,
		NotBefore:     time.Now(), NotAfter: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM signing_keys WHERE kid = $1`, kid)
	})

	// ---- a PII key, sealed with AAD = "pii:" || identity_id ---------------
	tenantID, identityID := seedIdentity(t, pool, n)
	piiMaterial := randomKey(t)
	sealedPII, err := keyring.SealSecret(oldMaster, "pii:"+identityID, piiMaterial)
	if err != nil {
		t.Fatal(err)
	}
	piiKeyID, err := idStore.CreatePIIKey(ctx, tenantID, sealedPII, "old-master")
	if err != nil {
		t.Fatal(err)
	}
	if err := idStore.SetIdentityPIIKey(ctx, tenantID, identityID, piiKeyID); err != nil {
		t.Fatal(err)
	}

	// ---- an operator TOTP secret, sealed with AAD = the operator's id -----
	opName := fmt.Sprintf("zzreseal%d", n%1_000_000_000)
	opID, err := ctlStore.CreatePlatformUser(ctx, opName, opName+"@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM platform_users WHERE id = $1::uuid`, opID)
	})
	totpSecret := randomKey(t)
	if err := ctlStore.StageTOTPSecret(ctx, oldMaster, opID, totpSecret); err != nil {
		t.Fatal(err)
	}

	// ---- rewrap, through the SAME AAD functions the sweep uses -----------
	//
	// Only this test's own rows: the sweep is global, and a shared database
	// already holds keys sealed under the real master, which this test does
	// not have and must not disturb.
	rewrap := func(sealed []byte, aad string) []byte {
		material, err := keyring.OpenSecret(oldMaster, aad, sealed)
		if err != nil {
			t.Fatalf("probe secret did not open under the old master: %v", err)
		}
		out, err := keyring.SealSecret(newMaster, aad, material)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	mine, err := keyStore.ResealableKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var resealedKey bool
	for _, k := range mine {
		if k.Kid != kid {
			continue
		}
		if err := keyStore.ResealKey(ctx, k.ID, rewrap(k.Sealed, signingKeyAAD(k.Kid))); err != nil {
			t.Fatal(err)
		}
		resealedKey = true
	}
	if !resealedKey {
		t.Fatal("ResealableKeys did not list the probe signing key")
	}

	pks, err := idStore.ResealablePIIKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var resealedPII bool
	for _, p := range pks {
		if p.KeyID != piiKeyID {
			continue
		}
		if p.IdentityID != identityID {
			t.Fatalf("pii key %s reported identity %s, want %s", p.KeyID, p.IdentityID, identityID)
		}
		if err := idStore.ResealPIIKey(ctx, p.KeyID, rewrap(p.Sealed, piiKeyAAD(p.IdentityID)), "new-master"); err != nil {
			t.Fatal(err)
		}
		resealedPII = true
	}
	if !resealedPII {
		t.Fatal("ResealablePIIKeys did not list the probe key — the join to identities is wrong")
	}

	tts, err := ctlStore.ResealableTOTPSecrets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var resealedTOTP bool
	for _, tt := range tts {
		if tt.IDText != opID {
			continue
		}
		if err := ctlStore.ResealTOTPSecret(ctx, tt.ID, rewrap(tt.Sealed, operatorTOTPAAD(tt.IDText))); err != nil {
			t.Fatal(err)
		}
		resealedTOTP = true
	}
	if !resealedTOTP {
		t.Fatal("ResealableTOTPSecrets did not list the probe operator")
	}

	// ---- read every shape back, under the NEW master ----------------------
	records, err := keyStore.VerificationKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range records {
		if r.Kid != kid {
			continue
		}
		found = true
		got, err := keyring.OpenSecret(newMaster, r.Kid, r.PrivateKeyEnc)
		if err != nil {
			t.Fatalf("signing key will not open under the new master: %v", err)
		}
		if string(got) != string(seed) {
			t.Fatal("the signing key came back changed")
		}
	}
	if !found {
		t.Fatal("the probe signing key vanished")
	}

	_, sealedNow, _, err := idStore.IdentityAttributes(ctx, tenantID, identityID)
	if err != nil {
		t.Fatal(err)
	}
	gotPII, err := keyring.OpenSecret(newMaster, "pii:"+identityID, sealedNow)
	if err != nil {
		t.Fatalf("pii key will not open under the new master: %v", err)
	}
	if string(gotPII) != string(piiMaterial) {
		t.Fatal("the pii key came back changed")
	}

	gotTOTP, err := ctlStore.TOTPSecret(ctx, newMaster, opID)
	if err != nil {
		t.Fatalf("operator totp will not open under the new master: %v", err)
	}
	if string(gotTOTP) != string(totpSecret) {
		t.Fatal("the operator totp secret came back changed")
	}

	// And the old master must no longer work, or "rewrapped" is unproven.
	if _, err := ctlStore.TOTPSecret(ctx, oldMaster, opID); err == nil {
		t.Fatal("the old master still opens the secret — nothing was rewrapped")
	}
}

func randomKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func seedIdentity(t *testing.T, pool *pgxpool.Pool, n int64) (tenantID, identityID string) {
	t.Helper()
	ctx := context.Background()
	slug := fmt.Sprintf("zzrs%d", n%1_000_000_000)
	if err := pool.QueryRow(ctx,
		`INSERT INTO tenants (slug, name) VALUES ($1, 'Reseal probe') RETURNING id`,
		slug).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	var realmID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO realms (tenant_id, code, kind, display_name)
		 VALUES ($1, 'probe', 'internal', 'Probe') RETURNING id`,
		tenantID).Scan(&realmID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO identities (tenant_id, realm_id, username, status)
		 VALUES ($1, $2, $3, 'active') RETURNING id`,
		tenantID, realmID, slug).Scan(&identityID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c := context.Background()
		for _, q := range []string{
			`UPDATE identities SET pii_key_id = NULL WHERE tenant_id = $1::uuid`,
			`DELETE FROM pii_keys WHERE tenant_id = $1::uuid`,
			`DELETE FROM identities WHERE tenant_id = $1::uuid`,
			`DELETE FROM realms WHERE tenant_id = $1::uuid`,
			`DELETE FROM tenants WHERE id = $1::uuid`,
		} {
			_, _ = pool.Exec(c, q, tenantID)
		}
	})
	return tenantID, identityID
}
