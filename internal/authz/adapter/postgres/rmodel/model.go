// Package authzrmodel declares the authz tables storm generates code for.
//
// It is a PROJECTION of the schema, not its source of truth: anubis's schema
// of record stays migrations/ (forward-only, checksummed), and this model is
// validated against the live schema the same way the raw queries are — by
// preparing generated statements against it, and by the integration suite.
// Columns and defaults here must match `\d` on the table exactly.
package authzrmodel

import "github.com/gsoultan/storm"

// Role is public.roles. The id default is uuidv7() in the migration; storm's
// Model declares gen_random_uuid(), which is irrelevant here because storm
// emits no DDL for anubis — the masked insert simply leaves id unset and the
// database's own default fires.
type Role struct {
	storm.Model

	TenantID          storm.UUID
	ApplicationID     *storm.UUID
	IsSystem          bool
	Name              string
	Description       string
	AssignableAt      []string
	AllowedRealmKinds []string
}

func (r *Role) Schema(t *storm.Table) {
	t.Unique(&r.TenantID, &r.Name)
	t.Col(&r.Description).Default("''")
	t.Col(&r.AssignableAt).Default("'{}'")
	t.Col(&r.AllowedRealmKinds).Default("'{internal}'")
}

// All is what the generator consumes.
func All() []any { return []any{&Role{}} }
