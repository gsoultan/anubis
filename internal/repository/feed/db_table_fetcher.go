package feed

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/gsoultan/anubis/internal/domain"
	"github.com/gsoultan/anubis/internal/repository"
)

// DBTableFetcher reads a whole table from an external database, mapping its
// columns onto the feed contract:
//
//	{"dsn": "postgres://reader:...@crm-db:5432/crm",
//	 "table": "public.customers",
//	 "columns": {"ref": "id", "parent_ref": "parent_id", "name": "display_name",
//	             "node_type": "kind"}}          // node_type optional
//
// Table and column names come from admin-gated config but are STILL treated
// as hostile: validated against a strict identifier grammar and quoted, so a
// poisoned config cannot smuggle SQL into the external system.
type DBTableFetcher struct{}

func NewDBTableFetcher() *DBTableFetcher { return &DBTableFetcher{} }

type dbTableConfig struct {
	DSN     string            `json:"dsn"`
	Table   string            `json:"table"`
	Columns map[string]string `json:"columns"`
}

var identRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,62}$`)

func (f *DBTableFetcher) Fetch(ctx context.Context, source repository.SyncSourceRecord) ([]repository.SyncFeedRow, error) {
	var cfg dbTableConfig
	if err := json.Unmarshal(source.Config, &cfg); err != nil || cfg.DSN == "" || cfg.Table == "" {
		return nil, domain.ErrInvalidArgument.With("config", "db_table source needs dsn and table")
	}
	refCol, nameCol := cfg.Columns["ref"], cfg.Columns["name"]
	if refCol == "" || nameCol == "" {
		return nil, domain.ErrInvalidArgument.With("columns", "ref and name mappings are required")
	}

	table, err := quoteTable(cfg.Table)
	if err != nil {
		return nil, err
	}
	sel := []string{
		quoted(refCol) + " AS ref",
		quoted(nameCol) + " AS name",
	}
	orderBy := quoted(refCol)
	if c := cfg.Columns["parent_ref"]; c != "" {
		sel = append(sel, quoted(c)+" AS parent_ref")
		// parents before children, best effort: roots (NULL parent) first
		orderBy = quoted(c) + " NULLS FIRST, " + quoted(refCol)
	}
	if c := cfg.Columns["node_type"]; c != "" {
		sel = append(sel, quoted(c)+" AS node_type")
	}
	for _, c := range cfg.Columns {
		if !identRe.MatchString(c) {
			return nil, domain.ErrInvalidArgument.With("columns", "invalid identifier "+c)
		}
	}
	query := "SELECT " + strings.Join(sel, ", ") + " FROM " + table + " ORDER BY " + orderBy

	return fetchExternal(ctx, cfg.DSN, query)
}

func quoteTable(t string) (string, error) {
	parts := strings.Split(t, ".")
	if len(parts) > 2 {
		return "", domain.ErrInvalidArgument.With("table", t)
	}
	for _, p := range parts {
		if !identRe.MatchString(p) {
			return "", domain.ErrInvalidArgument.With("table", t)
		}
	}
	return pgx.Identifier(parts).Sanitize(), nil
}

func quoted(ident string) string {
	return pgx.Identifier{ident}.Sanitize()
}
