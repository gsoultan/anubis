package database

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// A guard's own sentence reaches the caller; PostgreSQL's never does.
func TestMapErrSayingCarriesOnlyRaisedMessages(t *testing.T) {
	reason := func(err error) string { return apperr.AsError(MapErrSaying(err)).Details["reason"] }

	raised := fmt.Errorf("exec: %w", &pgconn.PgError{Code: "23514", Message: `items of level "a" already sit under a "b"`})
	if got := reason(raised); got != `items of level "a" already sit under a "b"` {
		t.Fatalf("a raised guard message was dropped: %q", got)
	}
	if kind := apperr.AsError(MapErrSaying(raised)).Kind; kind != apperr.KindInvalidArgument {
		t.Fatalf("kind = %v, want invalid argument", kind)
	}

	// A declared CHECK: the server wrote this message and its detail holds the row.
	declared := &pgconn.PgError{Code: "23514", ConstraintName: "scope_nodes_slug_check",
		Message: `new row for relation "scope_nodes" violates check constraint "scope_nodes_slug_check"`}
	if got := reason(declared); got != "" {
		t.Fatalf("a declared constraint's message reached the caller: %q", got)
	}
	// Anything that is not a refusal by the schema stays opaque.
	for _, code := range []string{"23505", "23503", "42P01", "54000"} {
		if got := reason(&pgconn.PgError{Code: code, Message: "driver text"}); got != "" {
			t.Fatalf("SQLSTATE %s leaked its message: %q", code, got)
		}
	}
	if MapErrSaying(nil) != nil {
		t.Fatal("nil must stay nil")
	}
}
