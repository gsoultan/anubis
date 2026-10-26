// osiris2anubis reads the Osiris IAM tables and writes what Anubis imports:
//
//	manifest.json              permissions and roles — AuthzAdminService/ApplyManifest
//	rules.csv                  Osiris rule name → Anubis permission key, for CheckRule
//	people-and-access.xlsx     people and grants — ProvisioningService/ImportWorkbook
//	report.txt                 counts, and every decision the conversion had to make
//
// It writes files and nothing else: applying is a separate, reviewed step
// (the import has a dry run, the manifest has one too). Password hashes are
// deliberately not exported — a file of hashes is a file somebody mails —
// and move through IdentityAdminService/ImportPasswordHash at cut-over.
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gsoultan/anubis/cmd/osiris2anubis/convert"
	"github.com/gsoultan/anubis/cmd/osiris2anubis/pgsource"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "osiris2anubis:", err)
		os.Exit(1)
	}
}

func run() error {
	o := convert.DefaultOptions()
	var (
		dsn   = flag.String("dsn", os.Getenv("OSIRIS_DB_URL"), "Osiris PostgreSQL DSN (or OSIRIS_DB_URL); read-only")
		out   = flag.String("out", "osiris-export", "directory to write into")
		realm = flag.String("realms", "EMPLOYEE=internal,DEVICE=internal,APPLICANT=public,REGISTRANT=public",
			"entity_type=realm pairs; a type left out is skipped")
	)
	flag.StringVar(&o.App, "app", o.App, "Anubis application slug the manifest belongs to")
	flag.StringVar(&o.AxisOrg, "axis-org", o.AxisOrg, "structure code Osiris organisations were synced into")
	flag.StringVar(&o.AxisUnit, "axis-unit", o.AxisUnit, "structure code Osiris units were synced into")
	flag.StringVar(&o.AxisWorkplace, "axis-workplace", o.AxisWorkplace, "structure code Osiris workplaces were synced into")
	flag.BoolVar(&o.UnplacedEverywhere, "unplaced-everywhere", false,
		"grant people with no org/unit/workplace EVERYWHERE (Osiris's behaviour; the widest grant there is)")
	flag.Parse()
	if *dsn == "" {
		return fmt.Errorf("-dsn (or OSIRIS_DB_URL) is required")
	}
	o.Realms = map[string]string{}
	for _, pair := range strings.Split(*realm, ",") {
		t, r, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && t != "" && r != "" {
			o.Realms[strings.ToUpper(t)] = r
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	src, err := pgsource.Open(ctx, *dsn)
	if err != nil {
		return err
	}
	defer src.Close()
	catalog, err := src.Catalog(ctx)
	if err != nil {
		return err
	}
	people, err := src.People(ctx)
	if err != nil {
		return err
	}

	rep := convert.NewReport()
	cat := convert.Catalog(o.App, catalog, rep)
	pr := convert.People(o, cat, catalog, people, rep)

	if err := os.MkdirAll(*out, 0o750); err != nil {
		return err
	}
	manifest, err := json.MarshalIndent(cat.Manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, "manifest.json"), append(manifest, '\n'), 0o640); err != nil {
		return err
	}
	if err := writeRules(filepath.Join(*out, "rules.csv"), cat.Rules); err != nil {
		return err
	}
	wb, err := os.OpenFile(filepath.Join(*out, "people-and-access.xlsx"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if err := convert.WriteWorkbook(wb, pr); err != nil {
		_ = wb.Close()
		return err
	}
	if err := wb.Close(); err != nil {
		return err
	}
	report := rep.String()
	if err := os.WriteFile(filepath.Join(*out, "report.txt"), []byte(report), 0o640); err != nil {
		return err
	}
	fmt.Print(report)
	fmt.Printf("\nwrote %s/{manifest.json, rules.csv, people-and-access.xlsx, report.txt}\n", *out)
	return nil
}

func writeRules(path string, rules []convert.RuleMapping) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{"rule", "permission", "kind"})
	for _, r := range rules {
		kind := "permission"
		if r.Marker {
			kind = "scope_marker"
		}
		_ = w.Write([]string{r.Rule, r.Permission, kind})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
