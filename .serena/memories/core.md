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
