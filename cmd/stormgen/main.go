// Command stormgen is this module's storm tool.
//
//	ANUBIS_DB_URL=... go run ./cmd/stormgen generate \
//	    internal/authz/adapter/postgres/rgen -raw-schema live
//
// It is small because storm's commands are a library: they need to see THIS
// module's models, and a binary installed from storm's repository cannot.
// Everything else — verify -stale, verify -pending, lint, explain — comes with
// them, against THIS schema.
//
// -raw-schema live is required and deliberate: the models in rmodel are a
// PROJECTION, migrations/ is this schema's source of truth, and the raw
// declarations call SQL functions (authorize, membership_*,
// ensure_month_partitions) that no model describes. Validating them against a
// scratch apply of the model would fail every one.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	auditrmodel "github.com/gsoultan/anubis/internal/audit/adapter/postgres/rmodel"
	auditrquery "github.com/gsoultan/anubis/internal/audit/adapter/postgres/rquery"
	authrmodel "github.com/gsoultan/anubis/internal/auth/adapter/postgres/rmodel"
	authrquery "github.com/gsoultan/anubis/internal/auth/adapter/postgres/rquery"
	authzrmodel "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rmodel"
	authzrquery "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rquery"
	controlrmodel "github.com/gsoultan/anubis/internal/control/adapter/postgres/rmodel"
	controlrquery "github.com/gsoultan/anubis/internal/control/adapter/postgres/rquery"
	gaterquery "github.com/gsoultan/anubis/internal/gate/adapter/postgres/rquery"
	platformrquery "github.com/gsoultan/anubis/internal/platform/database/rquery"
	scopermodel "github.com/gsoultan/anubis/internal/scope/adapter/postgres/rmodel"
	scopermquery "github.com/gsoultan/anubis/internal/scope/adapter/postgres/rquery"
	tenancyrmodel "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/rmodel"
	tenancyrquery "github.com/gsoultan/anubis/internal/tenancy/adapter/postgres/rquery"
	"github.com/gsoultan/storm"
	"github.com/gsoultan/storm/tool"
)

// declarations is one bounded context's model and raw-query surface.
type declarations struct {
	models  []any
	queries []storm.RawDecl
}

// contexts is the registry. A context appears here exactly when it owns a
// rmodel/rquery pair; the ones still on sqlc are absent, which is the whole
// record of how far the migration has got.
var contexts = map[string]declarations{
	"authz":   {authzrmodel.All(), authzrquery.Queries()},
	"audit":   {auditrmodel.All(), auditrquery.Queries()},
	"control": {controlrmodel.All(), controlrquery.Queries()},
	"tenancy": {tenancyrmodel.All(), tenancyrquery.Queries()},
	"scope":   {scopermodel.All(), scopermquery.Queries()},
	"auth":    {authrmodel.All(), authrquery.Queries()},
	// No models: the gate owns no table. It READS eight of them to freeze a
	// snapshot, and modelling another context's tables here would put their
	// shape in this package's hands.
	"gate": {nil, gaterquery.Queries()},
	// No models: the technical context's two statements are advisory locks,
	// which belong to no table.
	"platform": {nil, platformrquery.Queries()},
}

func main() {
	name, ok := contextFrom(os.Args[1:])
	if !ok {
		fmt.Fprintf(os.Stderr,
			"stormgen: no bounded context in the arguments — the output path names it,\n"+
				"          as in internal/audit/adapter/postgres/rgen\n")
		os.Exit(2)
	}
	d, known := contexts[name]
	if !known {
		fmt.Fprintf(os.Stderr,
			"stormgen: %q is not a storm context yet (have: %s)\n"+
				"          add its rmodel/rquery pair to cmd/stormgen before generating\n",
			name, strings.Join(names(), ", "))
		os.Exit(2)
	}
	// Per context, not all at once: each owns its own generated package, and
	// handing storm every model would let one context's query compile against
	// another's table — the import boundary AGENTS.md draws, enforced where
	// the code is produced rather than after it exists.
	tool.Main(d.models, d.queries)
}

// contextFrom reads the bounded context out of the output path, so the command
// line keeps the shape it had with one context and gains no flag to forget.
func contextFrom(args []string) (string, bool) {
	const root = "internal/"
	for _, a := range args {
		i := strings.Index(a, root)
		if i < 0 {
			continue
		}
		rest := a[i+len(root):]
		if j := strings.IndexByte(rest, '/'); j > 0 {
			return rest[:j], true
		}
	}
	return "", false
}

func names() []string {
	out := make([]string, 0, len(contexts))
	for k := range contexts {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
