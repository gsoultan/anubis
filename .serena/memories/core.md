# Anubis — core memory

Multi-tenant IAM/SSO backend. Postgres 18 schema is the validated engine
(`authorize()` in SQL, 0.045 ms/decision); Go application layer wraps it.

## Load-bearing decisions (see docs/adr/)
- ADR-0002: no third-party libs except infra drivers. Crypto = stdlib only.
  PASETO/TOTP/migration runner hand-written. `internal/domain` imports stdlib only.
- ADR-0008: Connect RPC (connect-go v1.20, TS runtime v2) + go-kit endpoints.
  Two surfaces: Connect for RPC, stdlib net/http for OIDC/PKCE, well-known,
  gate check, health, hosted login. One server on :7448.
- ADR-0009: all SQL in db/queries/*.sql via sqlc → internal/adapter/postgres/gen.
  No SQL strings in hand-written Go (CI-enforced).
- pkg/anubis = nested zero-dependency Go module (verifier SDK). PASETO lives
  there; the server imports it for signing.

## Layout
cmd/anubisd (serve|migrate|keys) · internal/{config,domain,usecase,port,
endpoint,adapter/{postgres,connectapi,httpapi},crypto/{kdf,localtoken,keyring,
totp},migrate,ratelimit,snapshot} · pkg/anubis · proto/anubis/v1 · db/queries
· gen/go · migrations/ (forward-only, tracked in schema_migrations
version+sha256 checksum — must stay compatible with scripts/db.sh).

## Sharp edges
- authorize() semantics: OR within axis (bool_or, migration 0013), AND across
  axes, fail-closed on strict axes and `_owner` self-scope. Any in-memory
  evaluator must match it and be differentially tested against it.
- Snapshot loads: single REPEATABLE READ read-only tx or torn reads.
- Ports 7447 console / 7448 api / 7449 db, defined in scripts/lib/common.sh.
- Refresh reuse ⇒ revoke family + session, and that event must alert.
- Dev DB via Apple Container: scripts/db.sh; bench/rebuild.sh is the DB suite.

## Application layer (shipped in this build)
Phases 0–7 implemented and live-verified; e2e suite in test/e2e (integration
tag) covers login uniformity, refresh theft (incl. successor death — the
revocations must commit OUTSIDE the failed claim tx), authorize/explain,
logout, gate decisions, rate limiting. Enforcement: scripts/check/* wired in
.gitlab-ci.yml. Bootstrap: `anubisd bootstrap` seeds tenant/realm/admin/
apps/anubis.admin role (pattern anubis:*). Gate snapshot differential parity
with authorize() still to add as an automated test.

## Scope sync from external systems
Each axis has ONE source of truth (UNIQUE tenant_id, axis_code):
kind = http | db_query | db_table, config carries url/dsn — the fetcher
(internal/repository/feed) opens the SOURCE's own connection, never the
Anubis pool. Non-obvious invariants learned by building it:
- feeds MUST be sorted parents-first in Go (repository.SortFeedParentsFirst);
  no SQL ORDER BY expresses a topological order.
- RunSync must EnsureAxisRoot first or parentless rows violate
  nonroot_has_parent.
- unreachable feed => error; an empty feed would archive the whole axis.
- only external_ref-carrying nodes are archived; manual nodes are never
  sync's to remove (verified against ~31k seeded nodes).
- migration 0021 fixed dry runs resolving parents created in the same run.
- db_table builds SQL from validated+quoted identifiers only; ADR-0009
  records that exemption (foreign schemas sqlc cannot check).

## Structure (ADR-0010, refactored 2026-08-23)
Seven bounded contexts under internal/: identity, auth, authz, scope,
tenancy, audit, gate — each with domain/ port/ app/ service/ endpoint/
adapter/{postgres,rpc,http}. Plus shared/ (apperr, authctx, clock, validate,
jsonx, txm) and platform/ (config, crypto/*, database, migrate, mw,
ratelimit). Conventions that surprise newcomers:
- FOLDER names the layer, PACKAGE clause is context-prefixed and unique
  (internal/identity/domain => package identitydomain). Deliberate: no
  import aliases, and no package shadows locals like `identity`/`session`.
- <=10 Go files per folder, CI-enforced (scripts/check/folder-size.sh).
  Outgrowing it means a missing concept: that is why authz/domain/grant,
  authz/domain/membership and auth/app/{signin,mfa,device,session,token}
  exist.
- One sqlc package PER CONTEXT (db/queries/<ctx> -> internal/<ctx>/adapter/
  postgres/gen). No generated package holds another context's SQL.
- One repository type per context over platform/database (pool, WithinTx via
  context, MapErr + column helpers). The old god-Store is gone.
- internal/api/{connect,http} is transport plumbing and must NOT import any
  context; contexts register their own routes via srv.Handle (the compiler
  caught the cycle when this was violated).
- cmd/anubisd/application.go is the composition root: every wiring decision
  lives there, nothing else knows the whole system.

## Production readiness (2026-08-23)
Runtime: request/read/header timeouts + MaxHeaderBytes + body limits + panic
recovery in internal/api/http/server.go (NO WriteTimeout on purpose — it
would cut gRPC streams; per-request deadlines do that job, streaming
detected by content-type). Pool config in cmd serve.go (MaxConns default
4xGOMAXPROCS, lifetime jitter) + statement_timeout/application_name baked
into cfg.PoolURL(). /readyz fails on stale snapshot (gate fails closed past
maxAge, so the instance must leave the LB).
Jobs (internal/platform/jobs + cmd/anubisd/maintenance.go): partitions
(boot+daily), one-time sweep, retention, key-expiry warning; each under
pg_try_advisory_lock so replicas cooperate without leader election.
DB roles: migration 0023 (anubis_owner/app/readonly). app cannot CREATE or
UPDATE/DELETE audit_log; readonly cannot read credentials/keys/pii_keys.
Gotcha: bench/rebuild.sh is DESTRUCTIVE — after it run `anubisd baseline`
then `anubisd bootstrap` or every e2e login fails with invalid_credentials.
Second factors: an ENROLLED factor is always demanded (realm allow-list
still applies). Enrol-or-deny for required-but-unenrolled is deliberately
NOT implemented (policy flip would lock out existing users).
TOTP codes are single-use (last_step in credentials.params) — tests must
wait for the step boundary, not generate a future code (skew=1).
