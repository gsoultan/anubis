// Package syncengines declares which databases this build can read a scope
// structure out of.
//
// Its own package because cmd/anubisd sits at the ten-file limit the
// architecture enforces — the same reason the installer has one — and
// because the blank driver imports want somewhere obvious to live.
package syncengines

import (
	// The engines a structure feed may be read from. A driver's blank import
	// and its dialect registration belong together: a scheme that resolves to
	// a driver nobody linked in fails at the first sync, in production, on
	// somebody else's schedule.
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/gsoultan/anubis/internal/scope/adapter/feed"
)

// registerSyncEngines declares which databases this build can read a scope
// structure out of.
//
// This is the whole list, and it is deliberately in the composition root
// rather than inside the feed package: which engines an installation trusts
// itself to connect to is a deployment decision, and adding one should be a
// visible edit here — two lines, an import and a registration — not a change
// to the sync path.
//
// To add SQL Server:
//
//	_ "github.com/microsoft/go-mssqldb"
//	feed.RegisterDialect("sqlserver", feed.SQLServerDialect("sqlserver"))
//
// Anything with a database/sql driver works the same way. The dialect only
// has to describe how the engine quotes an identifier and how it sorts NULLs
// first; everything else is database/sql's problem.
func Register() {
	// pgx's database/sql driver is registered as "pgx". Both spellings of the
	// scheme are in the wild and operators write either.
	feed.RegisterDialect("postgres", feed.PostgresDialect("pgx"))
	feed.RegisterDialect("postgresql", feed.PostgresDialect("pgx"))

	feed.RegisterDialect("mysql", feed.MySQLDialect("mysql"))
	// MariaDB speaks the MySQL protocol and uses the same driver.
	feed.RegisterDialect("mariadb", feed.MySQLDialect("mysql"))
}
