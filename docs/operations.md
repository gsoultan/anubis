# Operations

Everything an on-call engineer needs at 3am, and nothing that duplicates
[architecture.md](architecture.md) or [security.md](security.md).

## Contents

1. [Deploying](#deploying)
2. [Configuration](#configuration)
3. [Database roles](#database-roles)
4. [Health and readiness](#health-and-readiness)
5. [Metrics](#metrics)
6. [TLS and the reverse proxy](#tls-and-the-reverse-proxy)
7. [Running more than one instance](#running-more-than-one-instance)
8. [Connecting a structure feed](#connecting-a-structure-feed-to-another-system)
9. [Machine credentials for pipelines](#machine-credentials-for-pipelines)
10. [Maintenance jobs](#maintenance-jobs)
11. [Key rotation](#key-rotation)
12. [Incident: refresh token reuse](#incident-refresh-token-reuse)
13. [Incident: signing key compromise](#incident-signing-key-compromise)
14. [Restoring from backup](#restoring-from-backup)
15. [Performance budgets](#performance-budgets)

---

## Deploying

Before pushing anything that will become a release, the whole pipeline runs
locally: `scripts/ci/local.sh` (or `--quick` to skip the suites). It performs
the same five stages CI does, in the same order — which is also how you keep
working when the runner is unavailable.

```bash
anubisd migrate          # schema, as anubis_owner — a separate step, on purpose
anubisd keys init access # FIRST INSTALL ONLY: mint the signing key
anubisd serve            # runtime, as anubis_app
```

**A fresh production install has no signing key.** Automatic key generation
is off outside dev on purpose — nothing mints signing material behind your
back — so until `keys init` runs, `/readyz` answers 503 and every login
fails. `keys init` refuses once an active key exists, so it cannot rotate a
live installation by accident; rotation is `prepare` then `promote`, below.

Migrations run **forward-only** and take an advisory lock, so several replicas
starting at once cannot race. `serve` in a non-prod environment applies
migrations itself; production does not, because a schema change should be a
deliberate deploy step with its own rollback plan (which is: roll forward).

The image (`Dockerfile`) is distroless and non-root, and embeds the
migrations **and the admin console** — nothing is mounted. The console is
served by `anubisd` itself at `/` on the API origin (same-origin by design;
the CORS knob is dev-only), so deploying the binary deploys the console.
A binary built without `scripts/build.sh` serves a placeholder page that
says how to get the real one.

## Configuration

| Variable | Default | Notes |
| :--- | :--- | :--- |
| `ANUBIS_DB_URL` | *required* | Runtime connects as `anubis_app` |
| `ANUBIS_LISTEN` | `:7448` | |
| `ANUBIS_ISSUER` | `http://localhost:7448` | The `iss` claim; **must** match what SDKs verify |
| `ANUBIS_ENV` | `dev` | `prod` refuses to boot without a master key |
| `ANUBIS_MASTER_KEY` | *required in prod* | base64url, 32 bytes, KMS-held. Unseals signing and PII keys |
| `ANUBIS_DB_MAX_CONNS` | `4 × GOMAXPROCS` | Size to the DATABASE, not the app |
| `ANUBIS_DB_STATEMENT_TIMEOUT` | `15s` | Server-side; a slower query is a bug |
| `ANUBIS_REQUEST_TIMEOUT` | `30s` | Streaming RPCs are exempt |
| `ANUBIS_MAX_REQUEST_BYTES` | `1MiB` | |
| `ANUBIS_SHUTDOWN_GRACE` | `20s` | Drain before the audit queue closes |
| `ANUBIS_SNAPSHOT_MAX_AGE` | `5m` | Past this the gate fails closed **and readiness fails** |
| `ANUBIS_DEBUG_LISTEN` | *(off)* | pprof/expvar; bind loopback only |
| `ANUBIS_UI_ORIGIN` | *(off)* | Dev CORS only; production is same-origin |
| `ANUBIS_TRUSTED_PROXIES` | *(none)* | CIDRs whose `X-Forwarded-For` is believed. **Set this behind a TLS proxy** or every caller shares one rate-limit bucket |
| `ANUBIS_SYNC_DENY_HOSTS` | *(none)* | CIDRs a structure feed may not reach. Link-local is always refused |
| `ANUBIS_SYNC_ALLOW_LOOPBACK` | `0` | Let a feed reach 127.0.0.1 — development only |

**Rate limits are per instance.** Counters live in the process
(`internal/platform/ratelimit`), so N replicas enforce N times the published
allowance. That is a deliberate trade — see
[ADR-0012](adr/0012-rate-limits-across-replicas.md) for why, and for the
conditions that should reopen it. Size accordingly: to hold a real ceiling,
divide the target by the replica count.

**The master key is the whole system.** Losing it makes every signing key and
every PII key unreadable; leaking it is equivalent to leaking the signing keys
themselves. Keep it in a KMS, never in a CI variable — masked variables are not
a security boundary.

## Database roles

`migrations/0023` provisions least privilege:

| Role | May | May not |
| :--- | :--- | :--- |
| `anubis_owner` | own the schema, run migrations | — |
| `anubis_app` | read/write data, execute functions | CREATE/DROP/ALTER, UPDATE or DELETE on `audit_log` |
| `anubis_readonly` | read non-secret tables | read `credentials`, `signing_keys`, `refresh_tokens`, `one_time_tokens`, `pii_keys` |

The roles are created `NOLOGIN`; the deployment grants LOGIN and a password, so
no credential ever lands in a migration file.

`migrations/0037` revokes `USAGE ON SCHEMA public` from `PUBLIC`, so those
grants decide something. Until it, every role in the database reached the
schema through `PUBLIC` whether or not it had been granted anything — the
explicit grants in 0023 were describing an access path, not controlling it.

**This means any role you add later needs `GRANT USAGE ON SCHEMA public`
by name.** A reporting user or an integration that used to work by default
will now get `relation "…" does not exist`, which is Postgres reporting a
schema it may not use. That is the intended behaviour and the reason to grant
deliberately.

It closes access to the data, not knowledge that the tables exist:
`pg_catalog` stays world-readable, as it does in every Postgres, so an
ungranted role can still list table names.

## Health and readiness

| Endpoint | Fails when | Orchestrator should |
| :--- | :--- | :--- |
| `GET /healthz` | process is wedged | restart |
| `GET /readyz` | database unreachable, no active signing key, or **snapshot older than `ANUBIS_SNAPSHOT_MAX_AGE`** | remove from the load balancer |

Readiness includes snapshot age because past that age the gate fails closed:
the instance is denying traffic it should allow, and must stop receiving it.

## Metrics

Prometheus text format at `/metrics` on the **debug listener**
(`ANUBIS_DEBUG_LISTEN`) — beside pprof and expvar, off the public surface.
Families: `anubis_endpoint_requests_total{endpoint,code}` and
`anubis_endpoint_duration_seconds` (histogram) for every RPC,
`anubis_audit_events_total{action}` (this is where `token.reuse_detected`
pages from), `anubis_job_runs_total{job,result}`,
`anubis_gate_snapshot_loaded_timestamp_seconds{tenant}`, `anubis_db_pool_*`,
and `anubis_build_info{version}`.

Alert rules, thresholds and the runbook section each one points at live in
[alerting.md](alerting.md). If you wire up exactly one alert, make it
refresh-token reuse.

## TLS and the reverse proxy

Anubis does not terminate TLS. It serves plain HTTP on `ANUBIS_LISTEN` and
expects a proxy in front — which is not optional if anyone signs in through a
browser: the SSO and sign-out cookies use the `__Host-` prefix, and browsers
only accept that over HTTPS.

Two settings make a proxied deployment correct, and both are easy to miss:

**1. Name your proxies.** `ANUBIS_TRUSTED_PROXIES` is a comma-separated list
of CIDRs (or bare addresses) whose `X-Forwarded-For` is believed. Leave it
empty and the client IP is the PROXY's on every request — so the per-IP rate
limits stop bounding one caller and start bounding the entire installation,
which fails a busy morning rather than an attacker. Set it and the header is
believed from those peers only. The header is read RIGHT to left, taking the
first hop that is not itself a trusted proxy, so a client that prepends its
own `X-Forwarded-For` cannot choose its address.

```
ANUBIS_TRUSTED_PROXIES=127.0.0.1/32,10.0.0.0/8
```

**2. `ANUBIS_ISSUER` must be the public HTTPS URL**, not the internal one.
It is the `iss` claim every SDK verifies and the base for every page URL and
discovery document. Getting it wrong produces token failures that look like
a key problem.

### nginx

```nginx
server {
    listen 443 ssl;
    server_name auth.example.com;

    ssl_certificate     /etc/letsencrypt/live/auth.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/auth.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:7448;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Connect/gRPC needs HTTP/1.1 with upgrade support; the default
        # HTTP/1.0 upstream breaks streaming RPCs.
        proxy_http_version 1.1;
        proxy_set_header Connection "";

        # Long enough for the KDF on sign-in (~50ms) plus headroom, short
        # enough to fail before the client gives up.
        proxy_read_timeout 60s;
    }
}
```

### Caddy

Caddy sets `X-Forwarded-*` itself and manages certificates, so it is two
lines:

```
auth.example.com {
    reverse_proxy 127.0.0.1:7448
}
```

Either way, keep the debug listener (`ANUBIS_DEBUG_LISTEN`) on loopback and
**never** proxy it: it serves `/metrics` and pprof.

## Running more than one instance

Anubis is designed for it — nothing is stored in the process that another
instance needs — but until now nobody had watched two run together. Drill
performed 2026-08-25, two instances against one database:

| Behaviour | Observed |
| :--- | :--- |
| Maintenance jobs | Each boot job ran on **exactly one** instance; the other recorded `result="skipped"`. That is `pg_try_advisory_lock` working, and it is why there is no leader election |
| Revocation | An API key revoked on instance A was refused by instance B on the **very next request** (200 → 401), with no restart and no cache to expire |
| Rate limits | Instance A refused the 6th attempt; instance B accepted the same account immediately — the per-instance multiplier [ADR-0012](adr/0012-rate-limits-across-replicas.md) accepts on purpose |

So: add instances freely for throughput (one saturates near 11.8k
decisions/s), and remember that the published rate limits multiply by the
instance count. If a real per-account ceiling matters, divide the configured
limit by the number of instances.

The gate's snapshot is also per-instance, refreshed on a poll and by
LISTEN/NOTIFY. `/readyz` fails once a snapshot passes
`ANUBIS_SNAPSHOT_MAX_AGE`, which is what takes a stale instance out of the
load balancer rather than letting it deny traffic it should allow.

**Sizing it.** Every instance holds every tenant's snapshot, so plan memory by
scope size × tenant count, not by traffic. Measured through the real loader:

| tenant scope size | snapshot resident |
| :--- | :--- |
| 32k nodes | ~1.5 MB of scope index (plus grants/identities) |
| 1M nodes | **~92 MB** |

The poll only *rebuilds* a tenant whose catalog version moved; an unchanged
tenant costs a single indexed row read. So idle tenants are nearly free, and
the cost that scales is resident memory rather than poll work. See
[ADR-0015](adr/0015-scope-hierarchy-at-scale.md).

**Size for the rebuild peak, not the steady state.** A rebuild constructs the
new snapshot in full *before* swapping the pointer — that is what makes the
swap atomic and lock-free for readers, and it means both copies are live for
the duration. Observed on the dev dataset with a server left running:

```
idle                      21.9 MB RSS
during a rebuild         396.0 MB RSS
+15s                      91.7 MB
+30s                      21.3 MB   settled
```

Steady state is small because Go returns pages with `MADV_FREE`, so RSS
understates the live heap while the process is quiet — and *overstates*
nothing during the spike. Container limits are enforced against RSS, so an
instance sized to the idle figure will be OOM-killed on a rebuild. Budget for
the peak: roughly twice a tenant's snapshot on top of the resident set, and
remember a fresh instance builds every tenant at once.

This is the other reason the version gate matters. It does not only save CPU —
it removes most of these spikes, because a snapshot that has not changed is
never rebuilt. Watch
`anubis_gate_snapshot_refresh_total{result=~"rebuilt|verify"}` to know how
often you are actually paying it.

**Cold start.** The version gate only helps a *running* instance; a fresh one
has nothing to compare against and loads every tenant in full, serially. At
~340 ms per million-node tenant that is the startup cost to plan a rollout
around, and `/readyz` correctly fails throughout — the instance is denying
traffic until its snapshots exist, which is why it must not be in the
balancer yet. Roll instances one at a time.

If a tenant's snapshot no longer fits comfortably, shard tenants across
instances before reaching for anything cleverer — the gate is deliberately
share-nothing, so that is a routing change and no code.

## Connecting a structure feed to another system

A scope axis can be kept in step with wherever that structure actually lives
— an ERP, a CRM, a warehouse. Three kinds ship: `http`, `db_query` (your SQL)
and `db_table` (name a table and map its columns).

**Any engine.** The database kinds go through `database/sql` and pick the
engine from the DSN's scheme. Registered by default:

| Scheme | Engine |
| :--- | :--- |
| `postgres://`, `postgresql://` | PostgreSQL |
| `mysql://`, `mariadb://` | MySQL, MariaDB |

Adding another is two lines in `cmd/anubisd/syncengines` — the driver's blank
import and a `RegisterDialect`. A dialect only describes what genuinely
differs: how the engine quotes an identifier, and how it sorts NULL parents
first. `SQLServerDialect` is written and waiting for its driver.

Write the DSN as a URL whatever the engine; MySQL's driver wants a different
format and Anubis translates it, so operators do not have to know that.

**Any host, with one exception.** A feed names a host and Anubis connects to
it, which is the shape of every SSRF. Link-local addresses are refused
outright — `169.254.0.0/16` carries the cloud metadata service, and no
structure feed has ever lived there. Loopback is refused unless
`ANUBIS_SYNC_ALLOW_LOOPBACK=1`, because in production a feed pointed at
127.0.0.1 is a feed pointed at Anubis itself. `ANUBIS_SYNC_DENY_HOSTS` takes
CIDRs for an installation that wants its own internal ranges off limits.

The authority to configure a source is already high (`anubis:sync:admin`,
operators only), so this is defence in depth rather than the only defence.

**What the feed must return**, whatever the kind: columns or JSON fields
`ref` and `name`, optionally `parent_ref` and `node_type`. Rows are sorted
parents-first by Anubis after fetching, because no SQL `ORDER BY` expresses a
topological order and few JSON APIs bother.

## Machine credentials for pipelines

Administration is operator-only (ADR-0011), so a pipeline that applies an
application manifest authenticates as an operator, with a platform API key:

```bash
curl -X POST "$ANUBIS/anubis.v1.AuthzAdminService/ApplyManifest" \
  -H "Authorization: Bearer $ANUBIS_API_KEY" \
  -H "X-Anubis-Tenant: $TENANT" \
  -H 'content-type: application/json' \
  -d @manifest.json
```

Create one under **Platform users → API keys**. Facts worth knowing before
you put one in CI:

- The key **acts as its owner** and carries exactly their assignments,
  re-read on every call. It can never do more than they can, and revoking
  or disabling them stops the key at the same moment.
- **Every key expires**; 90 days is the ceiling and there is no unbounded
  option. A credential that administers the installation should not outlive
  the reason it was made.
- It is shown **once**. Only a hash is stored, so a lost key is replaced,
  not recovered.
- Revocation takes effect on the next request — the lookup index excludes
  revoked rows.
- `X-Anubis-Tenant` is required for anything tenant-scoped, exactly as for a
  human operator; without it the call fails `no_tenant_selected` rather than
  guessing which tenant was meant.

## Maintenance jobs

Started automatically by `serve`; coordinated across replicas by advisory
locks, so every instance can run them safely.

| Job | Interval | Why it matters |
| :--- | :--- | :--- |
| `partitions` | boot + daily | Rows landing in the DEFAULT partition defeat partitioning; retention becomes a bulk DELETE again |
| `sweep_one_time_tokens` | hourly | MFA/PKCE/nonce rows live seconds; the rest is bloat on a hot path |
| `retention` | 6h | Applies realm `default_retention`, anonymises past the deadline, **shreds the PII key** |
| `signing_key_expiry` | boot + 6h | Warns 14 days out; an expired active key stops every login |

## Key rotation

Publish before you sign — consumer caches must warm first:

```bash
anubisd keys prepare access     # mints a PENDING key; it appears in /.well-known immediately
# wait for consumer key caches (default 5 min Cache-Control) …
anubisd keys promote access     # active -> retiring, pending -> active
```

Keep the retiring key published until the longest-lived token signed with it
has expired (≤ the access TTL). Rotate every 30–90 days; the job warns at 14
days.

## Incident: refresh token reuse

`action=token.reuse_detected` in the audit log **means a refresh token was
stolen**. It is the highest-signal alert in the system: either the attacker
used a token the legitimate user already rotated, or the user rotated one the
attacker holds. Anubis has already revoked the whole family and the session.

1. Identify the session and identity from the audit entry.
2. `BumpTokenEpoch` on that identity — kills every outstanding access token
   immediately, not just the refresh chain.
3. Review `audit_log` for that `actor_id` around the event: what did the
   session do before detection?
4. Force credential rotation if the access predates the detection.

## Incident: signing key compromise

Total compromise: the attacker mints valid tokens for any user in any scope
and the audit log shows nothing wrong.

1. `anubisd keys prepare access && anubisd keys promote access`.
2. Set the compromised key `retired` (it disappears from discovery).
3. Bump `token_epoch` for **every** identity — the only way to invalidate
   tokens already signed with the old key.
4. Rotate the master key and re-seal, if the master itself may have leaked.

## Restoring from backup

A restore brings revoked refresh tokens back to life. `token_epoch` is the
mitigation, and it only works if the restore is **consistent**: restore
`identities` and `refresh_tokens` from the same snapshot, never mix.

After any restore, bump `token_epoch` for affected identities unless you can
prove the snapshot post-dates the last revocation. `test/integration`
exercises this case explicitly.

### The drill, performed

Run this before you need it. Performed 2026-08-25 against a real database,
`pg_dump -Fc` then `pg_restore` into a fresh one:

| Checked | Result |
| :--- | :--- |
| Schema and data survive | 33 migrations, identities, operators and signing keys all identical |
| Schema guards survive | The `attributes` CHECK still refuses plaintext in the restored copy |
| Revocations survive | Revoked API keys came back revoked (8 of 8), not live |
| The copy actually serves | `/readyz` 200 and an operator login succeeded against it |

The last row is the one worth dwelling on. `/readyz` passing means the
restored **sealed signing keys opened**, which is only true because the
master key was supplied separately — the same fact that makes a stolen
backup worthless. Restore the database somewhere without the master key and
the instance cannot start, by design.

A drill nobody has run is a procedure, not a capability. Repeat it whenever
the schema changes shape.

## Sign-in and sign-out pages

Each tenant publishes as many as it needs; each has its own URL
(`/p/{tenant}/{kind}/{slug}`). Exactly one per kind is the default that
`/v1/authorize` and `/v1/logout` fall back to — it cannot be deleted or
disabled without promoting another first, because those endpoints must always
have something to render.

**Browser flows need TLS.** The SSO and sign-out cookies use the `__Host-`
prefix, which browsers only accept over HTTPS. Outside `ANUBIS_ENV=prod`, and
only for a request that arrived without TLS, the same cookies are issued
un-prefixed and without `Secure` so local development works. Production always
gets the hardened form — including behind a TLS-terminating proxy, where the
request reaches Anubis over plain HTTP.

## Performance budgets

Measured, and enforced by tests rather than asserted in prose:

| Path | Budget | Where |
| :--- | :--- | :--- |
| `authorize()` through pgx | p95 < 2 ms *(measured 0.33 ms)* | `TestAuthorizeLatencyBudget` |
| Snapshot decision (gate) | sub-µs, zero alloc *(measured 300 ns)* | `BenchmarkEvaluate` |
| Offline token verify (SDK) | ~50 µs, no I/O *(measured 62 µs)* | `BenchmarkVerify` |
| Path normalise + match | µs *(measured 0.9 + 0.8 µs)* | `BenchmarkNormalizePath`, `BenchmarkMatch` |
| Login | KDF-dominated *(~49 ms)* — **do not "optimise"** | `TestLoginTimingDoesNotRevealUserExistence` |
| Decision API under load | p99 < 50 ms at 32-way concurrency *(measured 5.6 ms)* | `TestAuthorizeUnderConcurrency` |

### What the API layer actually does under load

`authorize()` is fast in the database and through pgx, but both measure one
caller at a time. `TestAuthorizeUnderConcurrency` drives the whole path —
interceptor, guard, endpoint middleware, pool — and the shape is worth
knowing before you size anything:

| Concurrent callers | Throughput | p50 | p99 |
| :--- | :--- | :--- | :--- |
| 32 | 11,800/s | 2.6 ms | 5.6 ms |
| 64 | 11,700/s | 5.2 ms | 10.4 ms |
| 128 | 12,100/s | 10.4 ms | 14.3 ms |
| 256 | 11,800/s | 21.3 ms | 28.3 ms |

Throughput is flat and latency rises linearly, which is the signature of a
saturated system: past roughly 32 concurrent callers an instance is queueing,
not going faster. **Scale out, not up** — adding callers to one instance buys
latency and nothing else. (Measured on a development machine against a local
database; treat the shape as the lesson and re-measure on your own hardware.)

Turn it up for a soak: `ANUBIS_LOAD_WORKERS`, `ANUBIS_LOAD_SECONDS`.

**Soak, performed.** ~300,000 decisions across two rounds: goroutines held
at exactly 17 throughout, and RSS returned to 26 MB when idle after a 83 MB
working peak. No goroutine leak, no connection-handler leak, no heap that
only grows. Note that the throughput figures from that run are worthless —
the machine was at load average 35 — which is the general lesson: leak
checks survive a noisy host, latency measurements do not.

The login number is a security property, not a performance problem: the KDF
cost is what makes offline cracking expensive, and it must be paid identically
whether or not the user exists.
