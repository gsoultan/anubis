package feed

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// The two database-backed source kinds. Both read from SOMEBODY ELSE'S
// database over its own connection — never Anubis's pool — and both go
// through database/sql, so the engine is whatever the DSN's scheme resolves
// to in the dialect registry.
//
//	db_query  {"dsn": "postgres://reader:…@erp-db:5432/erp",
//	           "query": "SELECT id AS ref, parent_id AS parent_ref, name FROM cost_centers"}
//
//	db_table  {"dsn": "mysql://reader:…@crm-db:3306/crm",
//	           "table": "customers",
//	           "columns": {"ref": "id", "parent_ref": "parent_id", "name": "display_name"}}
//
// db_query hands the statement over as written: it is the operator's SQL,
// against the operator's database, and Anubis is in no position to parse it.
// db_table BUILDS a statement, which is why identifiers there are validated
// against a strict grammar and then quoted by the engine's own rule.

type DBQueryFetcher struct{}

func NewDBQueryFetcher() *DBQueryFetcher { return &DBQueryFetcher{} }

type dbQueryConfig struct {
	DSN   string `json:"dsn"`
	Query string `json:"query"`
}

func (f *DBQueryFetcher) Fetch(ctx context.Context, source scopedomain.SyncSourceRecord) ([]scopedomain.SyncFeedRow, error) {
	var cfg dbQueryConfig
	if err := json.Unmarshal(source.Config, &cfg); err != nil || cfg.DSN == "" || cfg.Query == "" {
		return nil, apperr.ErrInvalidArgument.With("config", "db_query source needs dsn and query")
	}
	return fetchExternal(ctx, cfg.DSN, cfg.Query)
}

type DBTableFetcher struct{}

func NewDBTableFetcher() *DBTableFetcher { return &DBTableFetcher{} }

type dbTableConfig struct {
	DSN     string            `json:"dsn"`
	Table   string            `json:"table"`
	Columns map[string]string `json:"columns"`
}

// identRe is deliberately narrower than any engine allows. A name that needs
// more than this is a name Anubis declines to build a statement around.
var identRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,62}$`)

func (f *DBTableFetcher) Fetch(ctx context.Context, source scopedomain.SyncSourceRecord) ([]scopedomain.SyncFeedRow, error) {
	var cfg dbTableConfig
	if err := json.Unmarshal(source.Config, &cfg); err != nil || cfg.DSN == "" || cfg.Table == "" {
		return nil, apperr.ErrInvalidArgument.With("config", "db_table source needs dsn and table")
	}
	refCol, nameCol := cfg.Columns["ref"], cfg.Columns["name"]
	if refCol == "" || nameCol == "" {
		return nil, apperr.ErrInvalidArgument.With("columns", "ref and name mappings are required")
	}
	// Every mapped column, not just the ones used below: a bad identifier is
	// a bad config whether or not this run happens to interpolate it.
	for _, c := range cfg.Columns {
		if !identRe.MatchString(c) {
			return nil, apperr.ErrInvalidArgument.With("columns", "invalid identifier "+c)
		}
	}

	dialect, _, err := dialectFor(cfg.DSN)
	if err != nil {
		return nil, err
	}
	query, err := buildTableQuery(dialect, cfg)
	if err != nil {
		return nil, err
	}
	return fetchExternal(ctx, cfg.DSN, query)
}

// buildTableQuery assembles the statement in the engine's own spelling. Kept
// separate from Fetch so it can be tested without a database — the quoting
// and the ordering are the parts that silently differ per engine.
func buildTableQuery(d *Dialect, cfg dbTableConfig) (string, error) {
	table, err := quoteTable(d, cfg.Table)
	if err != nil {
		return "", err
	}
	refCol, nameCol := cfg.Columns["ref"], cfg.Columns["name"]
	sel := []string{
		d.QuoteIdent(refCol) + " AS ref",
		d.QuoteIdent(nameCol) + " AS name",
	}
	orderBy := d.QuoteIdent(refCol)
	if c := cfg.Columns["parent_ref"]; c != "" {
		sel = append(sel, d.QuoteIdent(c)+" AS parent_ref")
		orderBy = d.OrderParentsFirst(d, c, refCol)
	}
	if c := cfg.Columns["node_type"]; c != "" {
		sel = append(sel, d.QuoteIdent(c)+" AS node_type")
	}
	return "SELECT " + strings.Join(sel, ", ") + " FROM " + table + " ORDER BY " + orderBy, nil
}

func quoteTable(d *Dialect, t string) (string, error) {
	parts := strings.Split(t, ".")
	if len(parts) > 2 {
		return "", apperr.ErrInvalidArgument.With("table", t)
	}
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		if !identRe.MatchString(p) {
			return "", apperr.ErrInvalidArgument.With("table", t)
		}
		quoted = append(quoted, d.QuoteIdent(p))
	}
	return strings.Join(quoted, "."), nil
}

// fetchExternal opens the SOURCE's own connection, reads at most maxRows,
// and closes it again. Shared by both database kinds.
//
// The mapping is by COLUMN NAME, not position, so the operator's query may
// select whatever else it likes and in whatever order.
func fetchExternal(ctx context.Context, dsn, query string) ([]scopedomain.SyncFeedRow, error) {
	dialect, u, err := dialectFor(dsn)
	if err != nil {
		return nil, err
	}
	if err := allowExternalHost(u.Hostname()); err != nil {
		return nil, err
	}
	driverDSN, err := dialect.TranslateDSN(u)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, externalTimeout)
	defer cancel()

	db, err := sql.Open(dialect.Driver, driverDSN)
	if err != nil {
		return nil, apperr.ErrInvalidArgument.With("dsn", "unusable").Wrap(err)
	}
	defer db.Close()
	// One run, one connection: this is a batch read of somebody else's
	// database, not a pool we should be holding open between syncs.
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(externalTimeout)

	if err := db.PingContext(ctx); err != nil {
		return nil, apperr.ErrUnavailableFeed.Wrap(err)
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, apperr.ErrInvalidArgument.With("query", "external query failed").Wrap(err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, apperr.ErrUnavailableFeed.Wrap(err)
	}
	idx := map[string]int{}
	for i, c := range cols {
		idx[strings.ToLower(c)] = i
	}
	refI, okRef := idx["ref"]
	nameI, okName := idx["name"]
	if !okRef || !okName {
		return nil, apperr.ErrInvalidArgument.With("query", "must return columns ref and name")
	}
	parentI, hasParent := idx["parent_ref"]
	typeI, hasType := idx["node_type"]

	var out []scopedomain.SyncFeedRow
	cells := make([]any, len(cols))
	holders := make([]any, len(cols))
	for i := range cells {
		holders[i] = &cells[i]
	}
	for rows.Next() {
		if err := rows.Scan(holders...); err != nil {
			return nil, apperr.ErrUnavailableFeed.Wrap(err)
		}
		row := scopedomain.SyncFeedRow{
			Ref:  asString(cells[refI]),
			Name: asString(cells[nameI]),
		}
		if hasParent {
			row.ParentRef = asString(cells[parentI])
		}
		if hasType {
			row.NodeType = asString(cells[typeI])
		}
		out = append(out, row)
		if len(out) > maxRows {
			return nil, tooMany(len(out))
		}
	}
	return out, rows.Err()
}

// asString flattens whatever the driver handed back. Drivers disagree about
// the Go type behind a column — []byte here, string there, int64 for a
// numeric key — and a ref is a ref regardless.
func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case time.Time:
		return t.Format(time.RFC3339)
	default:
		b, _ := json.Marshal(t)
		s := string(b)
		if len(s) >= 2 && s[0] == '"' {
			s = s[1 : len(s)-1]
		}
		return s
	}
}
