package feed

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

// DBQueryFetcher runs an operator-configured query against an EXTERNAL
// database — its own connection from config.dsn, opened per run and closed
// after; Anubis's pool is never involved:
//
//	{"dsn": "postgres://reader:...@erp-db:5432/erp",
//	 "query": "SELECT id AS ref, parent_id AS parent_ref, name FROM cost_centers ORDER BY parent_id NULLS FIRST"}
//
// The query must yield columns ref, parent_ref, name and optionally
// node_type, parents before children.
type DBQueryFetcher struct{}

func NewDBQueryFetcher() *DBQueryFetcher { return &DBQueryFetcher{} }

type dbQueryConfig struct {
	DSN   string `json:"dsn"`
	Query string `json:"query"`
}

func (f *DBQueryFetcher) Fetch(ctx context.Context, source repository.SyncSourceRecord) ([]repository.SyncFeedRow, error) {
	var cfg dbQueryConfig
	if err := json.Unmarshal(source.Config, &cfg); err != nil || cfg.DSN == "" || cfg.Query == "" {
		return nil, domain.ErrInvalidArgument.With("config", "db_query source needs dsn and query")
	}
	return fetchExternal(ctx, cfg.DSN, cfg.Query)
}

// fetchExternal is shared by db_query and db_table: dedicated read-only-ish
// connection, bounded runtime, column-name-driven row mapping.
func fetchExternal(ctx context.Context, dsn, query string) ([]repository.SyncFeedRow, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	pcfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, domain.ErrInvalidArgument.With("dsn", "unparseable").Wrap(err)
	}
	// The external side must not be able to hold a sync worker hostage.
	pcfg.RuntimeParams["statement_timeout"] = "45000"
	pcfg.RuntimeParams["application_name"] = "anubis-scope-sync"

	conn, err := pgx.ConnectConfig(ctx, pcfg)
	if err != nil {
		return nil, domain.ErrUnavailableFeed.Wrap(err)
	}
	defer conn.Close(context.WithoutCancel(ctx))

	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, domain.ErrInvalidArgument.With("query", "external query failed").Wrap(err)
	}
	defer rows.Close()

	idx := map[string]int{}
	for i, fd := range rows.FieldDescriptions() {
		idx[string(fd.Name)] = i
	}
	refI, okRef := idx["ref"]
	nameI, okName := idx["name"]
	if !okRef || !okName {
		return nil, domain.ErrInvalidArgument.With("query", "must return columns ref and name")
	}
	parentI, hasParent := idx["parent_ref"]
	typeI, hasType := idx["node_type"]

	var out []repository.SyncFeedRow
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, domain.ErrUnavailableFeed.Wrap(err)
		}
		row := repository.SyncFeedRow{
			Ref:  asString(vals[refI]),
			Name: asString(vals[nameI]),
		}
		if hasParent {
			row.ParentRef = asString(vals[parentI])
		}
		if hasType {
			row.NodeType = asString(vals[typeI])
		}
		out = append(out, row)
		if len(out) > maxRows {
			return nil, tooMany(len(out))
		}
	}
	return out, rows.Err()
}

func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	default:
		b, _ := json.Marshal(t)
		s := string(b)
		if len(s) >= 2 && s[0] == '"' {
			s = s[1 : len(s)-1]
		}
		return s
	}
}
