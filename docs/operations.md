# Operations

Everything an on-call engineer needs at 3am, and nothing that duplicates
[architecture.md](architecture.md) or [security.md](security.md).

## Contents

1. [Deploying](#deploying)
2. [Configuration](#configuration)
3. [Database roles](#database-roles)
4. [Health and readiness](#health-and-readiness)
5. [Metrics](#metrics)
6. [Machine credentials for pipelines](#machine-credentials-for-pipelines)
7. [Maintenance jobs](#maintenance-jobs)
8. [Key rotation](#key-rotation)
9. [Incident: refresh token reuse](#incident-refresh-token-reuse)
10. [Incident: signing key compromise](#incident-signing-key-compromise)
11. [Restoring from backup](#restoring-from-backup)
12. [Performance budgets](#performance-budgets)

---

## Deploying

```bash
anubisd migrate     # schema, as anubis_owner — a separate step, on purpose
anubisd serve       # runtime, as anubis_app
```

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

The login number is a security property, not a performance problem: the KDF
cost is what makes offline cracking expensive, and it must be paid identically
whether or not the user exists.
