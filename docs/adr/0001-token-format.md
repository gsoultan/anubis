# ADR-0001 — Token format: PASETO v4.public, with JWS as a dormant hedge

**Status:** accepted · **Date:** 2026-08-22

## Context

Access tokens cross trust boundaries and are read by every application. The
format decision is expensive to reverse once tokens are in circulation and
consumer code depends on them.

## The case against JWT

The criticism is structural, not folklore:

1. **The attacker controls the algorithm.** `alg` lives in the header, which is
   attacker-supplied and parsed *before* verification. This produced two famous
   vulnerability classes — `alg: none` accepted as valid, and RS256→HS256
   confusion where a token is re-signed using the RSA *public* key as an HMAC
   secret.
2. **JOSE is a kitchen sink.** JWS + JWE + JWK + JWA admit RSA-PKCS1v1.5
   (Bleichenbacher), AES-CBC-HMAC (ordering bugs), ECDH-ES (invalid curve). A
   spec permitting this much guarantees divergent implementations, and divergence
   in cryptography is vulnerability.
3. **No canonical serialisation.** Field boundaries are ambiguous enough that
   confusion attacks exist between concatenated components.

## What PASETO changes

| JWT problem | PASETO answer |
| :--- | :--- |
| Negotiated `alg` | **Versioned protocol.** `v4.public` *is* the cipher suite. No algorithm field exists to attack. |
| Many algorithms, several unsafe | **Two purposes, one suite each.** `v4.public` = Ed25519. `v4.local` = XChaCha20-Poly1305. |
| Ambiguous serialisation | **PAE** — length-prefixed canonical encoding before signing. Kills the confusion-attack class structurally. |
| No context binding | **Implicit assertions** — bound into the signature, not carried in the token. |
| Encryption needs JWE | `v4.local` is straightforward AEAD. |

## The counter-case, stated honestly

**A correctly-pinned JWT verifier is not measurably weaker.** Hard-code
`ed25519.Verify` and never read `alg`, and algorithm confusion is gone. The CVEs
were library bugs, not mathematics. PASETO makes the safe path the only path —
real value, but ergonomic safety rather than cryptographic superiority.

**PASETO's real cost is platform lock-out, and it is underrated.** Nothing
off-the-shelf speaks PASETO:

- Envoy, Istio, Kong, Traefik, nginx `auth_jwt` — all validate JWT, none PASETO
- Every cloud API gateway authorizer — JWT only
- Every external federation partner and SaaS SSO integration — JWT/OIDC only
- Migrating to Keycloak, Auth0 or Zitadel later — JWT only

Today every consumer is an internal service we control, so this costs nothing.
In three years, when someone wants Envoy to reject unauthenticated traffic
*before* it reaches a pod, PASETO is a wall.

**PASERK is far less mature than JWKS.** Fewer implementations, less tooling.

**Token format is roughly 5% of the security.** Revocation, refresh rotation,
theft detection, session anchoring, audience validation and scope evaluation are
the other 95%. A perfect PASETO implementation with 24-hour access tokens and no
rotation is far less secure than a boring pinned-EdDSA JWT with 5-minute TTLs and
family revocation.

## Decision

**Do not choose globally — the three token types have different requirements.**

| Token | Choice | Why |
| :--- | :--- | :--- |
| Access | **PASETO `v4.public`** | Crosses boundaries, read by many, must be signed and readable |
| Refresh | **Opaque 256-bit random**, stored SHA-256 hashed | A token format here adds attack surface and buys nothing |
| Internal state | **`anb.local.v1`** — AES-256-GCM + HKDF-SHA256 | Carries sensitive state, only Anubis reads it. JWE would be malpractice. |

Implement both codecs behind one interface; ship PASETO, keep JWS tested and
disabled per-application:

```go
type TokenCodec interface {
    Sign(ctx context.Context, c Claims, key SigningKey) (string, error)
    Verify(ctx context.Context, token string, ks KeySet) (Claims, error)
    Format() string // "v4.public" | "jws.eddsa"
}
```

Roughly 200 lines each. One day of work hedges the only real risk.

## Go implementation

`v4.public` is **100% spec-compliant using the standard library alone** —
`crypto/ed25519` plus a ~25-line PAE encoder plus `encoding/base64`.

For local tokens, strict `v4.local` needs XChaCha20-Poly1305 and BLAKE2b, which
live in `golang.org/x/crypto`. Those tokens never leave Anubis, so spec
compliance buys nothing. We define `anb.local.v1` using AES-256-GCM +
HKDF-SHA256 — both standard library — keeping PASETO's discipline (version = full
cipher suite, no negotiation) at zero dependency cost.

## Consequences

**Positive** — no algorithm-confusion surface; zero third-party cryptography;
clean encrypted internal tokens; migration path preserved by the dormant codec.

**Negative** — no gateway or service-mesh can validate our tokens today; smaller
ecosystem; engineers must learn a less common format.

**Mitigation** — the JWS codec is built and tested from day one, enabled by a
per-application config flag.

## The trap PASETO does not solve

`kid` lives in the PASETO **footer**, read *before* verification — structurally
the same position as JWT's header `kid`.

> `kid` may only index a pre-loaded, bounded, in-memory map. Never a database
> query, never a filesystem path, never a network fetch. Unknown `kid` → reject
> immediately with zero I/O.

Attacker-controlled input driving a lookup is how you get injection and denial of
service.
