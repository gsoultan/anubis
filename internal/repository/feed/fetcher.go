// Package feed implements repository.ScopeFeedFetcher for the three source
// kinds of migrations/0017: http, db_query, db_table.
//
// External-database fetchers open their OWN connection from config.dsn —
// any Postgres anywhere, never Anubis's pool. The SQL here is either
// operator-configured (db_query) or assembled from strictly-validated,
// quoted identifiers (db_table); it targets foreign schemas that cannot be
// known at build time, which is why this package carries the same explicit
// exemption from the no-SQL-in-Go gate as the migration runner.
package feed

import (
	"context"
	"fmt"

	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

// Limits every fetcher enforces; a feed is attacker-adjacent input.
const (
	maxRows      = 50_000
	maxBodyBytes = 10 << 20 // http: 10 MiB
)

// Fetcher dispatches to the kind-specific implementations.
type Fetcher struct {
	http    *HTTPFetcher
	dbQuery *DBQueryFetcher
	dbTable *DBTableFetcher
}

func NewFetcher() *Fetcher {
	return &Fetcher{
		http:    NewHTTPFetcher(),
		dbQuery: NewDBQueryFetcher(),
		dbTable: NewDBTableFetcher(),
	}
}

var _ repository.ScopeFeedFetcher = (*Fetcher)(nil)

func (f *Fetcher) Fetch(ctx context.Context, source repository.SyncSourceRecord) ([]repository.SyncFeedRow, error) {
	var (
		rows []repository.SyncFeedRow
		err  error
	)
	switch source.Kind {
	case "http":
		rows, err = f.http.Fetch(ctx, source)
	case "db_query":
		rows, err = f.dbQuery.Fetch(ctx, source)
	case "db_table":
		rows, err = f.dbTable.Fetch(ctx, source)
	default:
		return nil, domain.ErrInvalidArgument.With("kind", source.Kind)
	}
	if err != nil {
		return nil, err
	}
	// No source can be trusted to emit parents first — SQL cannot express it
	// and a JSON API rarely bothers. Guarantee it here, once, for all kinds.
	return repository.SortFeedParentsFirst(rows), nil
}

func tooMany(n int) error {
	return domain.ErrInvalidArgument.With("rows", fmt.Sprintf("feed exceeds %d rows (%d)", maxRows, n))
}
