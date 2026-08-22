// Package migrations embeds the SQL migrations so anubisd carries its schema
// with it (ADR-0002: the migration runner is hand-written; `embed` is the
// stdlib mechanism that makes that practical).
package migrations

import "embed"

// FS holds every numbered migration.
//
//go:embed *.sql
var FS embed.FS
