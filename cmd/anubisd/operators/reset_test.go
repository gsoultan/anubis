package operators

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
	"github.com/jackc/pgx/v5/pgxpool"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// The break-glass reset has to work on a real database, because the thing it
// is for is a real installation where nobody can sign in. Everything it
// touches — the hash format, the epoch bump, the audit insert — is the
// database's opinion, not Go's.
func TestResetPasswordLetsALockedOutOperatorBackIn(t *testing.T) {
	dsn := os.Getenv("ANUBIS_DB_URL")
	if dsn == "" {
		t.Skip("ANUBIS_DB_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	username := fmt.Sprintf("breakglass-%d", time.Now().UnixNano())
	old, err := kdf.Hash("the-password-nobody-remembers")
	if err != nil {
		t.Fatal(err)
	}
	var id string
	var epochBefore int
	if err := pool.QueryRow(ctx,
		`INSERT INTO platform_users (username, email, password_hash, status)
		 VALUES ($1, $2, $3, 'active') RETURNING id, token_epoch`,
		username, username+"@example.test", old).Scan(&id, &epochBefore); err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM platform_users WHERE id = $1`, id)
	})

	if err := Run(ctx, quiet(), []string{"reset-password", username}); err != nil {
		t.Fatalf("reset: %v", err)
	}

	var got string
	var epochAfter int
	if err := pool.QueryRow(ctx,
		`SELECT password_hash, token_epoch FROM platform_users WHERE id = $1`, id).
		Scan(&got, &epochAfter); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got == old {
		t.Fatal("the password was not changed")
	}
	// The epoch is what ends the sessions. A reset that leaves them running
	// is the case this tool exists for, done badly.
	if epochAfter != epochBefore+1 {
		t.Fatalf("token_epoch %d -> %d, want +1: sessions opened with the old "+
			"password keep working", epochBefore, epochAfter)
	}
	// And the hash it wrote is one the login path can actually verify. A
	// break-glass tool that writes a malformed hash locks the account
	// permanently, which is worse than the state it was called to fix.
	if _, _, err := kdf.Verify("obviously-wrong", got); err != nil { // kdf-rehash-exempt: reading the format, not a login
		t.Fatalf("the stored hash is not readable by kdf.Verify: %v", err)
	}
}

// An operator who does not exist is an error, not a silent success. Somebody
// running this has usually mistyped a username, and a tool that says nothing
// leaves them believing an account was reset.
func TestResetPasswordRefusesAnUnknownOperator(t *testing.T) {
	if os.Getenv("ANUBIS_DB_URL") == "" {
		t.Skip("ANUBIS_DB_URL not set")
	}
	err := Run(context.Background(), quiet(),
		[]string{"reset-password", fmt.Sprintf("nobody-%d", time.Now().UnixNano())})
	if err == nil {
		t.Fatal("resetting an operator that does not exist reported success")
	}
}

func TestUsageIsRefusedWithoutAUsername(t *testing.T) {
	if err := Run(context.Background(), quiet(), []string{"reset-password"}); err == nil {
		t.Fatal("no username was accepted")
	}
	if err := Run(context.Background(), quiet(), nil); err == nil {
		t.Fatal("no subcommand was accepted")
	}
}
