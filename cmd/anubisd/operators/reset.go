package operators

import (
	"context"
	"fmt"
	"log/slog"

	auditpg "github.com/gsoultan/anubis/internal/audit/adapter/postgres"
	auditdomain "github.com/gsoultan/anubis/internal/audit/domain"
	controlpg "github.com/gsoultan/anubis/internal/control/adapter/postgres"
	"github.com/gsoultan/anubis/internal/platform/config"
	"github.com/gsoultan/anubis/internal/platform/crypto/kdf"
	"github.com/gsoultan/anubis/internal/platform/crypto/secret"
	"github.com/gsoultan/anubis/internal/platform/database"
	"github.com/gsoultan/anubis/internal/shared/jsonx"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Run is `anubisd operators reset-password <username>` — the way
// back in when nobody can sign in to let you.
//
// ResetOperatorPassword over the API needs another operator, already signed
// in, already holding PermAssignOperators. That covers the ordinary case and
// not the one that ends an installation: a single owner who has lost their
// password, with nobody left who can reset it for them. Before this, the
// answer was to open the database and write a hash by hand, which is a thing
// people do at 3am with no audit trail and no guarantee they got the format
// right.
//
// The authority here is shell on the host plus the database URL, which is
// strictly more than any token buys. That is the point of a break-glass tool
// rather than a weakness of one — but it is also why it leaves a record.
func Run(ctx context.Context, logger *slog.Logger, args []string) error {
	if len(args) < 2 || args[0] != "reset-password" {
		return fmt.Errorf("usage: anubisd operators reset-password <username>")
	}
	username := args[1]

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	db := database.New(pool)
	store := controlpg.New(db)

	who, _, err := store.PlatformUserByUsername(ctx, username)
	if err != nil {
		return err
	}
	if who == nil {
		return fmt.Errorf("no operator named %q", username)
	}

	temporary, err := secret.New(24)
	if err != nil {
		return err
	}
	hash, err := kdf.Hash(temporary)
	if err != nil {
		return err
	}
	// SetPassword advances token_epoch in the same statement, so anything
	// this account had open stops working now. Somebody running this because
	// an account was compromised needs exactly that.
	if err := store.SetPassword(ctx, who.ID, hash); err != nil {
		return err
	}

	// Audited under the installation tenant like every other platform event,
	// with an actor kind that does not pretend to be a person. Nobody was
	// signed in; what happened is that somebody with the host did this.
	auditor := auditpg.NewChainedAuditor(auditpg.New(db), logger)
	auditor.Emit(ctx, auditdomain.AuditEvent{
		TenantID:  auditdomain.InstallationTenant,
		ActorKind: "cli",
		TargetID:  who.ID,
		Action:    "platform.operator_password_reset",
		Result:    "allow",
		Detail: jsonx.Must(map[string]string{
			"username": who.Username,
			"surface":  "cli",
		}),
	})
	auditor.Close() // flushes; the process is about to exit

	fmt.Printf("temporary password for %s:\n\n  %s\n\n", who.Username, temporary)
	fmt.Println("Shown once. Every session this account had open has ended.")
	fmt.Println("Sign in with it, then change it from the console.")
	return nil
}
