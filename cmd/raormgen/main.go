// Command raormgen regenerates the authz context's raorm package.
//
//	ANUBIS_DB_URL=... go run ./cmd/raormgen
//
// Statements are PREPAREd against the LIVE dev schema — not a scratch apply of
// the model — because anubis's schema of record is migrations/, and the raw
// queries reference SQL functions (authorize, membership_*) and views the
// model deliberately does not describe. The model is a projection; the
// database is the truth it is checked against.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	authzrmodel "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rmodel"
	authzrquery "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rquery"
	"github.com/gsoultan/raorm"
	"github.com/gsoultan/raorm/codegen"
	"github.com/jackc/pgx/v5"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	dsn := os.Getenv("ANUBIS_DB_URL")
	if dsn == "" {
		return fmt.Errorf("raormgen needs ANUBIS_DB_URL: raw queries are validated against the live dev schema")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	s, err := raorm.Build(authzrmodel.All()...)
	if err != nil {
		return err
	}

	var scanners []codegen.RawScanner
	for i, d := range authzrquery.Queries() {
		rt, sql := raorm.DeclOf(d)
		name := "raorm.SQLExec"
		if rt != nil {
			name = "raorm.SQL[" + rt.Name() + "]"
		}
		sd, err := conn.Prepare(ctx, fmt.Sprintf("raormgen_%d", i), sql)
		if err != nil {
			return fmt.Errorf("%s does not prepare against the schema:\n  %w", name, err)
		}
		if rt == nil {
			if len(sd.Fields) > 0 {
				return fmt.Errorf(
					"%s returns %d column(s) — use raorm.SQL[T] to read them:\n%s",
					name, len(sd.Fields), sql)
			}
			continue
		}
		fields := make([]codegen.RawField, len(sd.Fields))
		for j, f := range sd.Fields {
			fields[j] = codegen.RawField{Name: string(f.Name), OID: f.DataTypeOID}
		}
		rs, err := codegen.ResolveRawScanner(rt, rt.PkgPath(), fields)
		if err != nil {
			return fmt.Errorf("%s\n  %w", name, err)
		}
		scanners = append(scanners, rs)
	}

	dir := filepath.Join("internal", "authz", "adapter", "postgres", "rgen")
	files, err := codegen.Package(s, codegen.PackageOptions{
		Dir:           dir,
		Import:        "github.com/gsoultan/raorm",
		Package:       "rgen",
		PackageImport: "github.com/gsoultan/anubis/internal/authz/adapter/postgres/rgen",
		RawScanners:   scanners,
	})
	if err != nil {
		return err
	}
	var paths []string
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, files[rel], 0o644); err != nil {
			return err
		}
		fmt.Printf("→ %s (%d bytes)\n", full, len(files[rel]))
	}
	return nil
}
