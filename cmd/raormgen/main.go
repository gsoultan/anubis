// Command raormgen is this context's raorm tool.
//
//	ANUBIS_DB_URL=... go run ./cmd/raormgen generate internal/authz/adapter/postgres/rgen -raw-schema live
//
// It is five lines because raorm's commands are a library: they need to see
// this module's models, and a binary installed from raorm's repository
// cannot. Everything else — verify -stale, verify -pending, lint, explain —
// comes with them, against THIS schema.
//
// -raw-schema live is required and deliberate: the model in rmodel is a
// PROJECTION, migrations/ is this schema's source of truth, and the raw
// declarations call SQL functions (authorize, membership_*) the model does
// not describe. Validating them against a scratch apply of the model would
// fail every one.
package main

import (
	authzrmodel "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rmodel"
	authzrquery "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rquery"
	"github.com/gsoultan/raorm/tool"
)

func main() { tool.Main(authzrmodel.All(), authzrquery.Queries()) }
