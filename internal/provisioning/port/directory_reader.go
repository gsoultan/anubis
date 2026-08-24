// Package provisioningport is what the importer needs from the rest of
// the system, stated as the narrowest interfaces that will do.
//
// Every one of these is satisfied structurally by an object the
// composition root already builds — a context's repository or its admin
// usecase — so cross-context traffic still goes through ports and domain
// types, and the importer stays testable against fakes a few lines long
// rather than against a database.
package provisioningport

import (
	"context"

	identitydomain "github.com/gsoultan/anubis/internal/identity/domain"
)

// DirectoryReader resolves the two names a workbook identifies a person
// by — the realm code and the username — into the ids the rest of the
// import needs.
type DirectoryReader interface {
	RealmByCode(ctx context.Context, tenantID, code string) (*identitydomain.Realm, error)
	IdentityForLogin(ctx context.Context, tenantID, realmID, username string) (*identitydomain.Identity, error)
}
