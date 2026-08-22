# ADR-0002 — Where the no-third-party-library line falls

**Status:** accepted · **Date:** 2026-08-22

## Context

The project requires no third-party libraries. Taken literally in Go this is
impossible — you cannot speak the Postgres wire protocol without a driver. Taken
loosely it is meaningless. The boundary must be explicit, or it gets re-argued in
every merge request.

## Decision

Three tiers, each with a different rule.

| Tier | Examples | Rule |
| :--- | :--- | :--- |
| **Cryptographic primitives** | SHA-256, HMAC, AES-GCM, Ed25519, HKDF | **Never hand-roll.** Standard library only. |
| **Protocol and format layers** | PASETO, PAE, TOTP, session handling, migration runner | **Hand-write.** Small, and understanding them is the point. |
| **Infrastructure drivers** | Postgres wire protocol, Redis, gRPC/protobuf | **Accepted exceptions.** Zero security benefit to reimplementing. |

Writing your own AES is the one decision that will actually cause a breach.
Writing your own PASETO signer over `crypto/ed25519` is ~150 lines and entirely
reasonable.

## What Go 1.26 provides

| Need | Source | Third-party? |
| :--- | :--- | :--- |
| Ed25519 sign/verify | `crypto/ed25519` | No |
| AEAD | `crypto/aes` + `crypto/cipher` (GCM) | No |
| Key derivation | `crypto/hkdf` *(stdlib since 1.24)* | No |
| Password hashing | `crypto/pbkdf2` *(stdlib since 1.24)* | No |
| Hashing | `crypto/sha256`, `crypto/sha3` | No |
| CSPRNG | `crypto/rand` | No |
| Constant-time compare | `crypto/subtle` | No |
| HTTP routing, methods + patterns | `net/http.ServeMux` *(since 1.22)* | No |
| Structured logging | `log/slog` *(since 1.21)* | No |
| Templating | `html/template` | No |
| Embedded migrations | `embed` | No |

**Result: zero third-party cryptography.** The only non-stdlib dependencies are
`pgx` and a Redis client, neither on a security-critical path.

## The one real gap: password hashing

Argon2id is the better KDF — memory-hard, GPU-resistant — and is **not** in the
standard library.

| Option | Assessment |
| :--- | :--- |
| `golang.org/x/crypto/argon2` | Best hash quality. Maintained by the Go team, but formally third-party. |
| **`crypto/pbkdf2` at ≥600k iterations** | **Chosen.** Standard library, OWASP-acceptable, zero dependencies. |
| Hand-write Argon2id from the RFC | **No.** Memory-hard function bugs are silent. |

**Design for the migration now.** Store algorithm and parameters *inside* the
hash string:

```
$pbkdf2-sha256$i=600000$<base64 salt>$<base64 hash>
```

Upgrading to Argon2id later is then a rehash on next successful login, with no
schema change. Reserve the format; the cost today is one decision.

## Rules that follow

1. **Ban `math/rand` repository-wide.** Lint it. This mistake has shipped in real
   auth systems.
2. **`internal/domain` imports nothing outside the standard library.** Enforced
   in CI.
3. **Every secret comparison uses `crypto/subtle` or `hmac.Equal`.** Never `==`.
4. **New dependencies require an ADR.** The list is short by design; keep it that
   way deliberately, not by accident.

## Consequences

**Positive** — a genuinely defensible dependency posture for a security service;
tiny supply-chain surface; the team understands its own cryptography.

**Negative** — more code to write and test; PBKDF2 is a weaker KDF than Argon2id;
some wheels reinvented.

**Accepted** — the reinvented wheels are format layers, where bugs are
discoverable by testing. The primitives, where bugs are silent, are not
reinvented.
