// Package feed implements scopeport.ScopeFeedFetcher: it reads a structure
// from wherever that organisation keeps it.
//
// Three things are deliberately open here, because the whole point is that
// the truth lives somewhere else:
//
//   - ANY KIND. Kinds are registered, not switched on. Adding one is
//     Register("ldap", f) and nothing in the sync path changes.
//   - ANY ENGINE. The database kinds go through database/sql and resolve the
//     engine from the DSN's scheme, so Postgres, MySQL, SQL Server or
//     anything else with a driver is a registration, not a code change.
//     RegisterDialect lives next to the driver's blank import.
//   - ANY HOST, with one exception that is not negotiable: see egress.go.
//     A feed at a cloud metadata endpoint is not a feed.
//
// External sources open their OWN connection from config.dsn — never
// Anubis's pool. The SQL is either operator-configured (db_query) or
// assembled from strictly-validated, engine-quoted identifiers (db_table);
// it targets foreign schemas that cannot be known at build time, which is
// why this package carries the same explicit exemption from the
// no-SQL-in-Go gate as the migration runner.
package feed

import (
	"context"
	"fmt"
	"strings"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	scopeport "github.com/gsoultan/anubis/internal/scope/port"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// Limits every fetcher enforces; a feed is attacker-adjacent input.
const (
	maxRows      = 50_000
	maxBodyBytes = 10 << 20 // http: 10 MiB
)

// KindFetcher reads one kind of source. Everything a kind must do beyond
// this — validating its own config, bounding its own transfer — belongs to
// the kind, because only it knows what its config means.
type KindFetcher interface {
	Fetch(ctx context.Context, source scopedomain.SyncSourceRecord) ([]scopedomain.SyncFeedRow, error)
}

// Fetcher routes to the kind named by the source. A map rather than a
// switch: a switch is a list of everything the world is allowed to be, kept
// in a file that has no opinion about it.
type Fetcher struct {
	kinds map[string]KindFetcher
}

// NewFetcher returns the kinds Anubis ships with. Register adds more.
func NewFetcher() *Fetcher {
	f := &Fetcher{kinds: map[string]KindFetcher{}}
	f.Register("http", NewHTTPFetcher())
	f.Register("db_query", NewDBQueryFetcher())
	f.Register("db_table", NewDBTableFetcher())
	return f
}

// Register adds or replaces a kind. The name must match
// scope_sync_sources.kind, which the schema constrains — so a kind is two
// changes, the CHECK and this, and neither works without the other.
func (f *Fetcher) Register(kind string, kf KindFetcher) {
	f.kinds[kind] = kf
}

// Kinds is what this installation can read, for an error message worth
// reading.
func (f *Fetcher) Kinds() []string {
	out := make([]string, 0, len(f.kinds))
	for k := range f.kinds {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

var _ scopeport.ScopeFeedFetcher = (*Fetcher)(nil)

func (f *Fetcher) Fetch(ctx context.Context, source scopedomain.SyncSourceRecord) ([]scopedomain.SyncFeedRow, error) {
	kf, ok := f.kinds[source.Kind]
	if !ok {
		return nil, apperr.ErrInvalidArgument.
			With("kind", source.Kind).
			With("registered", strings.Join(f.Kinds(), ", "))
	}
	rows, err := kf.Fetch(ctx, source)
	if err != nil {
		return nil, err
	}
	// No source can be trusted to emit parents first — SQL cannot express it
	// and a JSON API rarely bothers. Guarantee it here, once, for all kinds.
	return scopedomain.SortFeedParentsFirst(rows), nil
}

func tooMany(n int) error {
	return apperr.ErrInvalidArgument.With("rows", fmt.Sprintf("feed exceeds %d rows (%d)", maxRows, n))
}
