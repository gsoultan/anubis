package repository

import "context"

// ScopeFeedFetcher pulls a structure feed from wherever the truth lives —
// an HTTP API, a query against ANOTHER database, or a table in another
// database (scope_sync_sources.kind: http | db_query | db_table). The
// connection is the source's own (config.dsn / config.url), never Anubis's.
type ScopeFeedFetcher interface {
	Fetch(ctx context.Context, source SyncSourceRecord) ([]SyncFeedRow, error)
}
