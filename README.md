# Anubis

Single sign-on and identity & access management as a backend-only microservice.
Other applications authenticate and authorise against it over HTTP or gRPC.

Serves **all** identity populations from one service — internal employees,
external business partners (suppliers, vendors, contractors) and public users
(job applicants, customers) — with different authentication policy, identity
assurance, administration model and data-retention rules per population.

> Anubis weighs the heart at the gate. This service does the same for requests.

**Status:** database layer validated (20 migrations); Go application layer
running — Connect RPC + go-kit, PASETO tokens, browser SSO (PKCE), admin
plane, forward-auth gate from a REPEATABLE READ snapshot. See
[docs/roadmap.md](docs/roadmap.md).

---

## What it does

| Capability | Detail |
| :--- | :--- |
| **Sign in** | Password, device-bound biometric key, API key, TOTP — one pluggable credential model |
| **Tokens** | Short-lived signed access token + opaque rotating refresh token with theft detection |
| **Sign out** | Local (one device), global (all devices), and back-channel (notifies every app) |
| **Authorise** | Multi-axis scoped RBAC — organisation, application, department, work office, partner company, product, customer, and any dimension added later |
| **Populations** | Employees, partners and public users in one tenant, isolated by realm, with per-realm auth policy, assurance levels and retention |
| **Delegated admin** | Partners administer their own users; escalation paths closed in the schema |
| **Protect paths** | Route policies enforced in-app (SDK), at the gateway (nginx/Traefik/Envoy), or by a sidecar |
| **Audit** | Hash-chained, append-only, partitioned by month |

## The constraint that shapes everything

**No third-party libraries**, interpreted precisely — see [ADR-0002](docs/adr/0002-dependency-policy.md):

- **Cryptographic primitives** — never hand-rolled, always the Go standard library
- **Protocol and format layers** — hand-written (PASETO, TOTP, session handling)
- **Infrastructure drivers** — accepted as exceptions (`pgx`, Redis client)

Go 1.26's standard library makes this genuinely achievable. Anubis has
**zero third-party cryptography**.

## Installing a release

Linux, amd64 or arm64. The admin console is compiled into the binary, so
there is no second artefact to deploy.

```bash
# .deb / .rpm are published on the releases page, with a systemd unit
sudo dpkg -i anubisd_0.1.0_linux_amd64.deb

# The master key unseals every signing key and every encrypted column.
# Back it up somewhere that is not this host; losing it loses the data.
sudo install -d -m 0700 /etc/anubis/secrets
head -c 32 /dev/urandom | basenc --base64url | tr -d '=' \
  | sudo tee /etc/anubis/secrets/master.key >/dev/null
sudo chmod 0400 /etc/anubis/secrets/master.key

sudo -e /etc/anubis/anubisd.env          # ANUBIS_DB_URL, ANUBIS_ISSUER
anubisd migrate                          # schema, as the owner role
anubisd keys init access                 # FIRST INSTALL ONLY — without a
                                         # signing key /readyz is 503
sudo systemctl enable --now anubisd
```

Behind a TLS proxy — which browser sign-in requires, because `__Host-`
cookies do — also set `ANUBIS_TRUSTED_PROXIES`, or every caller shares one
rate-limit bucket. [operations.md](docs/operations.md) has worked nginx and
Caddy configs, and the runbook.

Releases ship an SBOM per archive and a signed `checksums.txt`. Verify
before installing:

```bash
cosign verify-blob checksums.txt \
  --signature checksums.txt.sig --certificate checksums.txt.pem \
  --certificate-identity-regexp 'https://github.com/gsoultan/anubis/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum -c checksums.txt --ignore-missing
```

## Quick start (development)

Requires [Apple Container](https://github.com/apple/container),
[Bun](https://bun.sh) 1.4.0, and Go 1.26+.

```bash
scripts/dev.sh          # database + api (when built) + console
```

| Script | Purpose |
| :--- | :--- |
| `scripts/dev.sh` | Whole environment. `--no-db`, `--ui-only`. |
| `scripts/ui.sh` | Console only, port 7447 |
| `scripts/api.sh` | API only, port 7448 |
| `scripts/db.sh` | `up · down · status · migrate · baseline · reset · recreate · psql` |
| `scripts/build.sh` | Build every workspace |
| `scripts/ci/local.sh` | The whole pipeline on this machine. `--quick` skips the suites. |
| `bench/rebuild.sh` | Migrate, seed and validate the database end to end |

### Ports

**7447** console · **7448** API · **7449** database — one contiguous block,
defined once in `scripts/lib/common.sh` and exported to every consumer.

Chosen against 3000/3001, 4200, 5000 (also macOS AirPlay Receiver), 5173,
8000/8080/8888, and everything already listening here: 5432 Postgres,
5672/15672 RabbitMQ, 6379 Redis, 7000, 9000/9001 MinIO. `strictPort` is on
everywhere — the neighbours are each other, so a silent fallback would land the
console on top of the API.

```
✗ port 7447 (console) is in use by: bun (pid 95701)
  override with: ANUBIS_UI_PORT=<port> scripts/ui.sh
```

### Validating the database

```bash
scripts/db.sh reset     # drop, migrate, seed, run every suite
scripts/db.sh status    # migrations applied, row counts, port publishing
```

Expected output:

```
==> dropping and rebuilding schema
    7 migrations applied
==> seeding
    150000 grants, 269751 grant_scopes, 128138 closure rows,
    57000 identities across 3 realms
==> correctness
     exact scope match -> ALLOW        | t   | t
     inherited descendant -> ALLOW     | t   | t
     different subtree -> DENY         | f   | f
     permission not held -> DENY       | f   | f
     FAIL-CLOSED: axis omitted -> DENY | f   | f
     cross-tenant identity -> DENY     | f   | f
==> external populations (suppliers, applicants)
     supplier reads own company PO -> ALLOW             | t   | t
     supplier reads ANOTHER company PO -> DENY          | f   | f
     applicant reads OWN record -> ALLOW                | t   | t
     applicant reads SOMEONE ELSE record -> DENY        | f   | f
     FAIL-CLOSED: self-scoped, no _owner -> DENY        | f   | f
     ASSURANCE: IAL1 applicant, IAL3 permission -> DENY | f   | f
     disabled identity -> DENY | f   | f
     anonymised (retention) identity -> DENY | f   | f
     same username across 3 realms |   3 |    3
    2/2 escalation attempts rejected
==> negative (all must be blocked by the schema)
    7/7 illegal writes rejected
==> performance
    20k decisions: Time: 891.963 ms
```

Full setup: [docs/development.md](docs/development.md).

Reporting a vulnerability: [SECURITY.md](SECURITY.md).

## Documentation

| Document | Contents |
| :--- | :--- |
| [architecture.md](docs/architecture.md) | System design, components, request flows |
| [schema.md](docs/schema.md) | Every table, column, index and constraint |
| [api.md](docs/api.md) | Complete endpoint reference |
| [integration.md](docs/integration.md) | How an application uses Anubis for authentication and authorization — the seven workflows, with diagrams |
| [security.md](docs/security.md) | Threat model and the controls answering each threat |
| [operations.md](docs/operations.md) | Deployment, monitoring, runbooks, incident response |
| [development.md](docs/development.md) | Local environment, migrations, testing |
| [benchmarks.md](docs/benchmarks.md) | Measured results and the methodology behind them |
| [roadmap.md](docs/roadmap.md) | Delivery phases |

### Decision records

| ADR | Decision |
| :--- | :--- |
| [0001](docs/adr/0001-token-format.md) | PASETO v4.public as primary, JWS/EdDSA as a dormant hedge |
| [0002](docs/adr/0002-dependency-policy.md) | Where the no-third-party-library line falls |
| [0003](docs/adr/0003-scope-model.md) | Forest of axes with closure tables, not one tree with paths |
| [0004](docs/adr/0004-authorization-semantics.md) | Evaluation rules and fail-closed guarantees |
| [0005](docs/adr/0005-database-performance.md) | Storage and query design, with measurements |
| [0006](docs/adr/0006-path-protection.md) | Three enforcement modes for securing website paths |
| [0007](docs/adr/0007-external-identities.md) | Realms for suppliers and applicants, not one tenant per partner |

## Layout

```
scripts/        platform orchestration: dev, build, database lifecycle.
migrations/     numbered, forward-only SQL. Applied in filename order.
bench/          reproducible correctness, negative and performance suites.
ui/             operator console (React 19 · Mantine 9 · Bun).
docs/           architecture, ADRs, operations.
```

## Knowledge & tooling

This repo participates in the Discover → Work → Verify → Persist loop:

| Tool | Role | Commands |
| :--- | :--- | :--- |
| **graphify** | Codebase knowledge graph — answer "where is X / what calls Y" from the graph, not by re-reading files | Build once: `/graphify . --code-only` · then `rtk graphify query "authorize"` · `graphify explain "<node>"` · after landing changes: `rtk graphify update .` |
| **Obsidian** | Durable decisions and incident notes | `rtk graphify export obsidian --dir ~/Documents/ObsidianVault/Anubis` |
| **skills.sh** | Reusable workflows — a manual workaround repeated three times becomes a skill | `rtk npx skills list` · `rtk npx skills add <skill>` |

The AST extraction cache is primed (`rtk graphify update .`); the first
`/graphify . --code-only` completes the graph at `graphify-out/graph.json`
(gitignored — it is derived, rebuild at will). Durable design decisions live in
`docs/adr/`; session-scoped notes belong in the vault, not the repo.

## License

MIT — © Gembit Soultan Shirazi <gembit.soultan@gmail.com>. See [LICENSE](LICENSE).
