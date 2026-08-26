# Development

## Prerequisites

| Tool | Version | Check |
| :--- | :--- | :--- |
| [Apple Container](https://github.com/apple/container) | 1.2+ | `container --version` |
| Go | 1.26+ | `go version` |
| macOS | 15+ on Apple Silicon | `sw_vers` |

Go 1.26 matters: `crypto/hkdf` and `crypto/pbkdf2` entered the standard library
in 1.24, and `net/http.ServeMux` gained method+pattern routing in 1.22. Together
they are what make the zero-third-party-crypto policy achievable —
[ADR-0002](adr/0002-dependency-policy.md).

## Database

Apple Container runs each container in a lightweight VM on Apple Silicon. The
CLI is Docker-shaped.

```bash
container system start          # once per boot

container run -d --name anubis-dev-pg \
  -e POSTGRES_PASSWORD=anubis \
  -e POSTGRES_USER=anubis \
  -e POSTGRES_DB=anubis \
  --cpus 4 --memory 4096M \
  docker.io/library/postgres:18-alpine \
  -c shared_buffers=1GB \
  -c effective_cache_size=3GB \
  -c work_mem=32MB \
  -c maintenance_work_mem=256MB \
  -c random_page_cost=1.1 \
  -c track_io_timing=on \
  -c log_min_duration_statement=100 \
  -c max_parallel_workers_per_gather=4
```

Those settings are not decoration:

| Setting | Why |
| :--- | :--- |
| `random_page_cost=1.1` | SSD. The default of 4.0 assumes spinning disks and pushes the planner away from index scans. |
| `track_io_timing=on` | Without it `EXPLAIN (BUFFERS)` cannot attribute I/O time. |
| `log_min_duration_statement=100` | Surfaces slow queries during development. |
| `shared_buffers=1GB` | Keeps the hot indexes resident; otherwise benchmarks measure page cache, not the design. |

**Postgres 18 specifically** — `uuidv7()` is native, and every primary key uses
it. See [ADR-0005 §1](adr/0005-database-performance.md).

### Connecting

Container networking is not exposed to the host by default, so drive it through
`container exec`:

```bash
container exec -it anubis-dev-pg psql -U anubis -d anubis
container exec -i  anubis-dev-pg psql -U anubis -d anubis < some.sql
```

Convenience shell function:

```bash
apsql() { container exec -i anubis-dev-pg psql -U anubis -d anubis "$@"; }
```

### Lifecycle

```bash
container ls -a                     # list
container stop  anubis-dev-pg
container start anubis-dev-pg
container rm -f anubis-dev-pg       # destroy (data is lost — no volume mounted)
```

---

## Migrations

Numbered, forward-only, applied in filename order. **No down-migrations** — a
rollback on an auth database is a data-loss event; roll forward instead.

```
migrations/
  0001_foundation.sql          extensions, tenants, applications, catalog_version
  0002_identity.sql            identities, credentials, sessions, tokens, keys
  0003_scope.sql               axes, node types, nodes, closure + maintenance
  0004_authz.sql               permissions, roles, role graph, grants
  0005_routes_audit.sql        route policies, audit log, partitioning
  0006_statement_triggers.sql  catalog invalidation (statement-level)
  0007_authorize.sql           the authorization decision function
```

Apply all:

```bash
for f in migrations/*.sql; do
  container exec -i anubis-dev-pg psql -U anubis -d anubis -v ON_ERROR_STOP=1 -q < "$f"
done
```

### Rules

1. **Expand/contract only.** Add nullable column → backfill → dual-write → switch
   reads → drop old. Never a destructive change in one step; you cannot take auth
   down for a schema change.
2. **Every migration is idempotent-safe to re-read** but applied once, tracked in
   `schema_migrations` with a checksum.
3. **Partitioned tables need partitions provisioned ahead.** `DEFAULT` partitions
   catch the gap, but the scheduled job must run.

---

## The validation suite

```bash
./bench/rebuild.sh
```

Drops the schema, reapplies every migration, seeds a realistic dataset, and runs
correctness, negative and performance suites. Roughly 30 seconds.

**It is destructive**, so anything bootstrapped is gone afterwards. To get a
working API back:

```bash
anubisd baseline    # records the rebuilt schema as applied
anubisd bootstrap --tenant impack --name Impack \
        --admin-user admin --admin-pass '<12+ chars>'
```

The Go suites layer on top:

```bash
go test -race -shuffle=on ./...                    # unit, fuzz corpora, benchmarks
go test -tags integration ./test/integration/      # claims + performance budgets
go test -tags integration ./test/e2e/              # against a running anubisd
```

```
==> dropping and rebuilding schema
    7 migrations applied
==> seeding
    150000 grants, 269491 grant_scopes, 128056 closure rows
==> correctness
     inherited descendant -> ALLOW     | t   | t
     different subtree -> DENY         | f   | f
     permission not held -> DENY       | f   | f
     FAIL-CLOSED: axis omitted -> DENY | f   | f
     cross-tenant identity -> DENY     | f   | f
==> negative (all must be blocked by the schema)
    7/7 illegal writes rejected
==> performance
    20k decisions: Time: 909.452 ms
```

| Script | Purpose |
| :--- | :--- |
| `bench/seed.sql` | 2 tenants, 3 axes, 32k scope nodes, 50k identities, 150k grants |
| `bench/run.sql` | Correctness — 5 cases including fail-closed |
| `bench/negative.sql` | 7 illegal writes that **must** be rejected by the schema |
| `bench/final.sql` | Throughput, storage profile, index usage |
| `bench/realms.sql` | External populations — suppliers, applicants, assurance gates, escalation guards |
| `bench/add_axis.sql` | Proves a new axis can be added at runtime without breakage |
| `bench/rebuild.sh` | All of the above |

### Seeding rules learned the hard way

**Never use `session_replication_role = 'replica'` to skip triggers.** It
disables *all* triggers, including data-integrity ones. This silently produced
`NULL` values in `permissions.key` and cost an hour. Disable specific triggers
instead — or better, do not depend on triggers for correctness
([ADR-0005 §9](adr/0005-database-performance.md)).

**`bench/realms.sql` is not idempotent** — it inserts roles. Run it once per
rebuild. Running it twice previously masked the escalation-guard count behind a
duplicate-key error.

**Make probes deterministic.** An early correctness probe took the first grant
via `LIMIT 1` and supplied only the `org` target. When the seed grew it selected
a grant that also constrained `product`, and the correct fail-closed deny looked
like a regression. Probes now build their target map from the grant's own
constraints.

**Avoid `ORDER BY md5(...)` inside a `LATERAL` for random picks.** It re-sorts
the candidate set once per outer row — O(n×m), 30 million hash computations at
this volume, and the seed times out. Index into a pre-aggregated array with
`hashtext()` instead.

---

## Benchmarking

`EXPLAIN` on an isolated subquery is not evidence. It led to a formulation that
looked **94× better in buffers** and was **17× slower end to end**
([ADR-0005 §4](adr/0005-database-performance.md)).

Rules:

1. **Benchmark the decision, not the subquery.**
2. **Compare variants that are identical in every other respect.** An early
   comparison omitted a clause from one variant and produced a meaningless
   result.
3. **Verify variants agree** before comparing their speed. Add a disagreement
   count to every comparison.
4. **Include the pathological case.** Broad grants (admins, auditors) are the
   common case, not an edge case — a workload of only narrow grants hides the
   difference entirely.

```sql
-- template
SELECT count(*) FILTER (WHERE a <> b) AS disagreements
FROM (SELECT variant_a(...) a, variant_b(...) b FROM workload) x;
```

---

## Go layout (planned)

```
cmd/anubisd/main.go
internal/
  domain/       entities + rules. ZERO imports outside stdlib. CI-enforced.
  usecase/      Login · Refresh · Logout · Authorize · SwitchScope · GateCheck
  port/         interfaces the domain needs (repos, clock, rand, notifier)
  adapter/
    postgres/   pgx. The ONLY place SQL exists.
    cache/      redis: rate limits, single-use nonces
    httpapi/    net/http ServeMux
    grpcapi/
  crypto/
    paseto/     v4.public (Ed25519, stdlib) + PAE encoder
    localtoken/ anb.local.v1 — AES-256-GCM + HKDF, stdlib
    kdf/        crypto/pbkdf2, versioned hash strings
    keyring/    in-memory kid→key map, rotation lifecycle
pkg/anubis/     public client SDK — ship with Phase 1
migrations/
```

### Testing requirements

| Target | Requirement |
| :--- | :--- |
| `internal/domain` | 100% unit tested, no database |
| Token parser | **Go fuzz target.** `FuzzVerifyToken` against malformed input. |
| PAE encoder | Property tests against the PASETO spec vectors |
| Path normaliser | Shared adversarial corpus run against **both** the gate and the app router, plus a fuzz target |
| `authorize` | The fail-closed case is the most important regression test in the project |
| Snapshot loader | Must assert `REPEATABLE READ`; a torn read is otherwise unreproducible |

### Lints that must be enforced

```
math/rand                     banned repository-wide
== on any secret              banned; use crypto/subtle
imports in internal/domain    stdlib only
```

## Building this repository elsewhere

`go.work` lists a sibling module by relative path:

```
use (
	.
	../raorm
	./pkg/anubis
)
```

That path exists on the machine that wrote it and nowhere else, so a fresh
clone — or CI — fails every `go` command with `cannot load module ../raorm
listed in go.work file`. `scripts/ci/fetch-workspace-modules.sh` clones the
siblings named there (pin with `ANUBIS_RAORM_REF`), and CI runs it before
anything else.

**The sibling must actually be published for that to work.** As of this
writing `gsoultan/raorm` is an empty repository — the code has never been
pushed — so nobody but its author can build this branch. Three ways out,
in increasing order of permanence:

1. **Push it.** The fetch script then works unchanged, and CI pins a ref.
2. **Tag it and depend on a version.** Drop the sibling from `go.work` and
   let `go.mod` require `github.com/gsoultan/raorm vX.Y.Z`. This is the
   normal answer once the module stops changing hourly.
3. **Vendor it**, if it is never meant to be consumed by anything else.

Until one of those, treat `go.work` as a local file: it is convenient for
whoever has both checkouts and an outright blocker for everyone else.
