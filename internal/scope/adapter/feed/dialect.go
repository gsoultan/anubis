package feed

import (
	"net/url"
	"strings"

	"github.com/gsoultan/anubis/internal/shared/apperr"
)

// A Dialect is the small set of things that genuinely differ between engines
// when Anubis reads a structure out of somebody else's database.
//
// It exists because db_table BUILDS a statement, and the two pieces it must
// build are exactly the two that are not portable: how an identifier is
// quoted, and how you sort roots before their children. Everything else —
// connecting, scanning, the column contract — is database/sql's job and is
// written once in sql_fetcher.go.
//
// Adding an engine is registering one of these plus its driver. Nothing in
// the sync path needs to learn about it.
type Dialect struct {
	// Driver is the database/sql driver name the caller has registered.
	Driver string

	// QuoteIdent renders one identifier safely. The identifier has already
	// been checked against a strict grammar; quoting is belt and braces, and
	// it is per-engine: "x" in Postgres, `x` in MySQL, [x] in SQL Server.
	QuoteIdent func(string) string

	// OrderParentsFirst sorts NULL parents first, which is the closest a
	// single query gets to topological order. Postgres and Oracle spell it
	// NULLS FIRST; MySQL and SQL Server have no such clause and need an
	// expression. Getting this wrong does not error — it silently returns
	// children before parents, which the reconciler then refuses row by row.
	OrderParentsFirst func(d *Dialect, parentCol, refCol string) string

	// TranslateDSN converts the URL an operator configured into whatever the
	// driver expects. Postgres drivers take the URL as-is; MySQL's does not.
	TranslateDSN func(*url.URL) (string, error)
}

// dialects is keyed by DSN scheme. Registering is how an installation gains
// an engine; nothing here imports a driver, so a deployment that never talks
// to MySQL never carries its code.
var dialects = map[string]*Dialect{}

// RegisterDialect adds an engine. Call it from the composition root, next to
// the blank import of the driver it names — the two belong together, and
// splitting them is how you get a scheme that resolves to a driver nobody
// registered.
func RegisterDialect(scheme string, d *Dialect) {
	dialects[strings.ToLower(scheme)] = d
}

// dialectFor resolves the engine from the DSN's scheme.
func dialectFor(dsn string) (*Dialect, *url.URL, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		return nil, nil, apperr.ErrInvalidArgument.
			With("dsn", "must be a URL naming the engine, e.g. postgres://… or mysql://…")
	}
	d, ok := dialects[strings.ToLower(u.Scheme)]
	if !ok {
		return nil, nil, apperr.ErrInvalidArgument.
			With("dsn", "no driver registered for "+u.Scheme).
			With("registered", strings.Join(registeredSchemes(), ", "))
	}
	return d, u, nil
}

func registeredSchemes() []string {
	out := make([]string, 0, len(dialects))
	for k := range dialects {
		out = append(out, k)
	}
	// Deterministic, so the error message is the same on every instance.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// --- the engines Anubis registers itself -----------------------------------

// doubleQuoted is the SQL standard, and what Postgres uses. An embedded
// quote is doubled; the identifier grammar already forbids one, so this
// only ever runs on input that cannot need it.
func doubleQuoted(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func backQuoted(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

func bracketQuoted(s string) string {
	return "[" + strings.ReplaceAll(s, "]", "]]") + "]"
}

// nullsFirstClause is the ANSI spelling: Postgres, Oracle, SQLite.
func nullsFirstClause(d *Dialect, parentCol, refCol string) string {
	return d.QuoteIdent(parentCol) + " NULLS FIRST, " + d.QuoteIdent(refCol)
}

// nullsFirstByExpression is for engines with no NULLS FIRST. Ordering by
// "is it null" descending puts roots first, which is the property the
// reconciler needs; MySQL and SQL Server both accept it.
func nullsFirstByExpression(d *Dialect, parentCol, refCol string) string {
	p := d.QuoteIdent(parentCol)
	return "CASE WHEN " + p + " IS NULL THEN 0 ELSE 1 END, " + p + ", " + d.QuoteIdent(refCol)
}

// PostgresDialect reads from PostgreSQL (and anything wire-compatible).
func PostgresDialect(driver string) *Dialect {
	return &Dialect{
		Driver:            driver,
		QuoteIdent:        doubleQuoted,
		OrderParentsFirst: nullsFirstClause,
		TranslateDSN:      func(u *url.URL) (string, error) { return u.String(), nil },
	}
}

// MySQLDialect reads from MySQL or MariaDB. Its driver does not take a URL,
// so the operator's mysql://user:pass@host:3306/db becomes the driver's own
// user:pass@tcp(host:3306)/db — the operator should not have to know that.
func MySQLDialect(driver string) *Dialect {
	return &Dialect{
		Driver:            driver,
		QuoteIdent:        backQuoted,
		OrderParentsFirst: nullsFirstByExpression,
		TranslateDSN: func(u *url.URL) (string, error) {
			host := u.Host
			if host == "" {
				return "", apperr.ErrInvalidArgument.With("dsn", "mysql needs a host")
			}
			var auth string
			if u.User != nil {
				if pw, ok := u.User.Password(); ok {
					auth = u.User.Username() + ":" + pw + "@"
				} else {
					auth = u.User.Username() + "@"
				}
			}
			dsn := auth + "tcp(" + host + ")/" + strings.TrimPrefix(u.Path, "/")
			if q := u.RawQuery; q != "" {
				dsn += "?" + q
			}
			return dsn, nil
		},
	}
}

// SQLServerDialect reads from Microsoft SQL Server, whose driver does take
// the sqlserver:// URL unchanged.
func SQLServerDialect(driver string) *Dialect {
	return &Dialect{
		Driver:            driver,
		QuoteIdent:        bracketQuoted,
		OrderParentsFirst: nullsFirstByExpression,
		TranslateDSN:      func(u *url.URL) (string, error) { return u.String(), nil },
	}
}
