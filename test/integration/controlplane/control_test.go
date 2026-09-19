//go:build integration

// Package controlplane exercises the control context against a real database.
//
// It exists because the context moved from sqlc to storm, and a query that
// COMPILES proves nothing about the rows it returns. The cases below are the
// ones where being wrong is not a wrong screen but a wrong decision: a
// case-insensitive sign-in lookup, the single-use TOTP guard, the token_epoch
// bump that makes "disabled" take effect now, and the single-use refresh
// rotation.
//
//	ANUBIS_DB_URL=postgres://anubis:anubis@localhost:7449/anubis?sslmode=disable \
//	  go test -tags integration ./test/integration/controlplane/
package controlplane

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	controlpg "github.com/gsoultan/anubis/internal/control/adapter/postgres"
	controldomain "github.com/gsoultan/anubis/internal/control/domain"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("ANUBIS_DB_URL")
	if dsn == "" {
		os.Exit(0) // nothing to test against; the e2e job sets this
	}
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		panic(err)
	}
	pool = p
	code := m.Run()
	// Probe operators only. Anything created here is prefixed, so the cleanup
	// cannot reach an operator somebody actually uses.
	_, _ = p.Exec(context.Background(),
		`DELETE FROM platform_users WHERE username LIKE 'zzprobe_%'`)
	p.Close()
	os.Exit(code)
}

func repo(t *testing.T) *controlpg.Repository {
	t.Helper()
	if pool == nil {
		t.Skip("ANUBIS_DB_URL not set")
	}
	return controlpg.New(database.New(pool))
}

func newOperator(t *testing.T, r *controlpg.Repository) (id, username string) {
	t.Helper()
	username = fmt.Sprintf("zzprobe_%d", time.Now().UnixNano())
	id, err := r.CreatePlatformUser(context.Background(), username, username+"@example.test", "hash")
	if err != nil {
		t.Fatalf("create operator: %v", err)
	}
	return id, username
}

// The sign-in lookup matches on lower(username), which is the expression the
// unique index is built on. A comparison that lost the lower() would still
// find the row typed exactly and fail for everybody else.
func TestPlatformUserLookupIsCaseInsensitive(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id, username := newOperator(t, r)

	for _, probe := range []string{username, upper(username), mixed(username)} {
		u, hash, err := r.PlatformUserByUsername(ctx, probe)
		if err != nil {
			t.Fatalf("lookup %q: %v", probe, err)
		}
		if u == nil {
			t.Fatalf("lookup %q found nothing", probe)
		}
		if u.ID != id {
			t.Fatalf("lookup %q returned %s, want %s", probe, u.ID, id)
		}
		if hash != "hash" {
			t.Fatalf("password hash came back %q", hash)
		}
	}

	// And a username that does not exist must find nothing, or "matched" and
	// "matched everything" are the same observation.
	if u, _, err := r.PlatformUserByUsername(ctx, "zzprobe_definitely_absent"); err != nil || u != nil {
		t.Fatalf("absent username returned (%v, %v)", u, err)
	}
}

func upper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 32
		}
	}
	return string(b)
}

func mixed(s string) string {
	b := []byte(s)
	if len(b) > 0 && b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 32
	}
	return string(b)
}

// Disabling must bump token_epoch in the SAME statement, computed by the
// database. In Go it is a read-modify-write, and two concurrent disables would
// lose one increment and leave an operator's tokens live.
func TestSetStatusBumpsTokenEpoch(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id, _ := newOperator(t, r)

	before, _, err := r.PlatformUserByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SetStatus(ctx, id, "disabled"); err != nil {
		t.Fatal(err)
	}
	after, _, err := r.PlatformUserByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.TokenEpoch != before.TokenEpoch+1 {
		t.Fatalf("token_epoch %d -> %d, want +1", before.TokenEpoch, after.TokenEpoch)
	}
	if after.Status != "disabled" || after.DisabledAt == nil {
		t.Fatalf("status=%q disabled_at=%v, want disabled and a timestamp", after.Status, after.DisabledAt)
	}

	// Re-enabling clears the timestamp and does NOT bump again: the CASE is
	// what makes those two different, and a lost CASE shows up here.
	if err := r.SetStatus(ctx, id, "active"); err != nil {
		t.Fatal(err)
	}
	back, _, err := r.PlatformUserByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if back.TokenEpoch != after.TokenEpoch {
		t.Fatalf("re-enabling bumped token_epoch to %d", back.TokenEpoch)
	}
	if back.DisabledAt != nil {
		t.Fatalf("re-enabling left disabled_at = %v", back.DisabledAt)
	}
}

// The single-use guard. A replayed code inside its own validity window must
// update nothing, and the comparison has to be in the WHERE: read-then-write
// would let both presentations pass the read.
func TestAdvanceTOTPStepIsSingleUse(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id, _ := newOperator(t, r)

	master := make([]byte, 32)
	if err := r.StageTOTPSecret(ctx, master, id, []byte("secret-bytes")); err != nil {
		t.Fatal(err)
	}
	// Staging must NOT enrol: holding a secret cannot start demanding a factor.
	staged, _, err := r.PlatformUserByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if staged.TOTPEnrolledAt != nil {
		t.Fatal("staging a secret enrolled the operator")
	}
	if err := r.ConfirmTOTP(ctx, id, 100); err != nil {
		t.Fatal(err)
	}

	ok, err := r.AdvanceTOTPStep(ctx, id, 101)
	if err != nil || !ok {
		t.Fatalf("first use of step 101: ok=%v err=%v", ok, err)
	}
	ok, err = r.AdvanceTOTPStep(ctx, id, 101)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("step 101 was accepted twice — the single-use guard is not guarding")
	}
	ok, err = r.AdvanceTOTPStep(ctx, id, 100)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("an older step was accepted")
	}

	// The sealed secret round-trips, which is what proves bytea survived.
	got, err := r.TOTPSecret(ctx, master, id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "secret-bytes" {
		t.Fatalf("totp secret came back %q", got)
	}
	if err := r.ClearTOTP(ctx, id); err != nil {
		t.Fatal(err)
	}
}

// Rotation: exactly one caller may consume a token. The second gets false,
// which the interactor reads as reuse.
func TestRefreshRotationIsSingleUse(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id, _ := newOperator(t, r)

	hash := make([]byte, 32)
	for i := range hash {
		hash[i] = byte(i)
	}
	familyID, err := r.CreateRefreshFamily(ctx, id, hash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	got, err := r.RefreshByHash(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("the token just written was not found by hash")
	}
	// The root's own id IS the family id, which is what one revocation kills.
	if got.ID != familyID || got.FamilyID != familyID {
		t.Fatalf("root id=%s family=%s, want both %s", got.ID, got.FamilyID, familyID)
	}

	ok, err := r.ConsumeRefresh(ctx, got.ID)
	if err != nil || !ok {
		t.Fatalf("first consume: ok=%v err=%v", ok, err)
	}
	ok, err = r.ConsumeRefresh(ctx, got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("a refresh token was consumed twice")
	}

	if err := r.RevokeRefreshFamily(ctx, familyID); err != nil {
		t.Fatal(err)
	}
	after, err := r.RefreshByHash(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if after == nil || after.RevokedAt == nil {
		t.Fatal("revoking the family did not mark the root revoked")
	}
}

// API keys: the lookup joins the owner, so a disabled operator's key stops
// working at the same moment they do.
func TestAPIKeyLookupCarriesOwnerStatus(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id, username := newOperator(t, r)

	lookup := fmt.Sprintf("zzk_%d", time.Now().UnixNano())
	keyID, err := r.CreatePlatformAPIKey(ctx, id, "probe", lookup, "sechash", "", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	auth, err := r.PlatformAPIKeyByLookup(ctx, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if auth == nil {
		t.Fatal("the key just minted was not found")
	}
	if auth.ID != keyID || auth.Username != username || auth.OwnerStatus != "active" {
		t.Fatalf("lookup returned %+v", auth)
	}

	if err := r.SetStatus(ctx, id, "disabled"); err != nil {
		t.Fatal(err)
	}
	auth, err = r.PlatformAPIKeyByLookup(ctx, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if auth == nil {
		t.Fatal("the key vanished when its owner was disabled; it should be found and refused")
	}
	if auth.OwnerStatus != "disabled" {
		t.Fatalf("owner status is %q after disabling", auth.OwnerStatus)
	}

	if err := r.RevokePlatformAPIKey(ctx, keyID); err != nil {
		t.Fatal(err)
	}
	if auth, err = r.PlatformAPIKeyByLookup(ctx, lookup); err != nil || auth != nil {
		t.Fatalf("a revoked key still resolves: (%v, %v)", auth, err)
	}
}

// Assignments: a global assignment has a NULL tenant, and the LEFT JOIN must
// keep it. An inner join would drop exactly the rows carrying most authority.
func TestGlobalAssignmentSurvivesTheJoin(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	id, _ := newOperator(t, r)

	aid, err := r.CreateAssignment(ctx, controldomain.AssignmentRecord{
		OperatorID: id, Role: controldomain.OperatorRole("owner"), Reason: "probe",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.RevokeAssignment(context.Background(), aid) }()

	mine, err := r.AssignmentsForOperator(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].TenantID != "" {
		t.Fatalf("operator assignments = %+v, want one with an empty tenant", mine)
	}

	all, err := r.Assignments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range all {
		if a.ID == aid {
			found = true
			if a.TenantID != "" || a.TenantSlug != "" {
				t.Fatalf("global assignment came back with tenant %q/%q", a.TenantID, a.TenantSlug)
			}
		}
	}
	if !found {
		t.Fatal("the global assignment was dropped by the join")
	}

	// A disabled operator's assignments disappear from the guard's lookup,
	// which is what makes disabling immediate.
	if err := r.SetStatus(ctx, id, "disabled"); err != nil {
		t.Fatal(err)
	}
	after, err := r.AssignmentsForOperator(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("a disabled operator still has %d assignments", len(after))
	}
}
