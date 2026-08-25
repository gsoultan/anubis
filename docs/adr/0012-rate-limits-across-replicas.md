# ADR-0012 — Rate limits across replicas

## Status

Accepted. Revisit when any of the trigger conditions below is met.

## Context

> Supersedes the `login_attempts` row in
> [ADR-0005](0005-database-performance.md#what-not-to-persist), which assumed
> Redis counters. There is no Redis; the counters are in-process.

`internal/platform/ratelimit` is a sharded in-memory token bucket. Counters
live in the process, so **each replica enforces the published limit
independently**: with N replicas behind a balancer, an attacker distributing
requests evenly gets N times the documented allowance.

The published limits are:

| Limit | Per minute | Keyed on |
| :--- | :--- | :--- |
| Tenant sign-in | 30 per IP | client address |
| Tenant sign-in | 10 per account | the target username |
| Platform sign-in | 10 per IP, 5 per account | address, username |
| Platform refresh | 30 per IP | client address |

The load harness (`TestAuthorizeUnderConcurrency`, and
[operations.md](../operations.md#performance-budgets)) shows one instance
saturating at roughly 11,800 decisions per second, which is where the replica
count comes from in the first place: a deployment adds instances to serve
throughput, and each one it adds also multiplies these ceilings.

The alternative is a shared counter store — Redis, or Postgres. ADR-0008's
posture is that no new infrastructure arrives until a deployment demands it,
and this is exactly the kind of decision that clause exists for.

## Decision

**Keep counters per-instance. Do not add a shared store yet.**

The reasoning is about what the limits are actually for.

The per-account limit is the one that stops credential stuffing, and its
value does not come from the exact number. Password verification pays a full
KDF (~49 ms, deliberately — see the login timing budget), so an attacker's
cost is dominated by that, not by our ceiling. Five replicas turn "10 guesses
a minute against one account" into 50: still four orders of magnitude short
of what offline cracking would give them, and every one of those attempts is
a row in the audit log with the target account named.

The per-IP limit bounds noise from one address. An attacker with enough
addresses to spread across replicas has already defeated a per-IP limit at
any multiplier, shared counters or not — that is what the account-keyed limit
is there to catch.

Meanwhile the cost of the shared store is real and permanent: a network hop
on the sign-in path, a new failure mode (what does the limiter do when Redis
is down — fail open and lose the limit, or fail closed and lose sign-in?),
and an operational dependency in a system whose entire deployment story is
currently "a binary and a database".

## Measured, not assumed

Two instances, one database, 2026-08-25: instance A refused the sixth
sign-in attempt for an account; instance B accepted that same account on the
next request. The multiplier is real and is exactly what this ADR accepts.
The same drill confirmed the two properties that make it tolerable —
maintenance jobs coordinate by advisory lock (one runs, the others skip) and
a revocation on one instance is refused by another on the very next request,
because authentication reads the database rather than a cache.

## Consequences

- **The documented limits are per instance.** [operations.md](../operations.md)
  and [security.md](../security.md) must say so plainly rather than quoting a
  number that is only true at one replica.
- **Alerting carries the weight** that shared counters would have. The
  `rate_limited` rate in [alerting.md](../alerting.md) is a campaign
  detector; unlike a counter, it works across replicas by construction
  because every instance reports to the same scraper.
- **Divide when sizing.** If a real per-account ceiling matters, set the
  per-instance limit to the target divided by the replica count, and accept
  that scaling out tightens it.

## Revisit when

Any one of these makes the trade wrong and the ADR should be reopened:

1. A deployment runs more than ~5 replicas, where the multiplier stops being
   a rounding error on a KDF-bound attack.
2. Redis (or an equivalent) arrives for another reason. Once the dependency
   and its failure modes are already owned, the argument against is mostly
   spent.
3. A limit is added that guards something CHEAP rather than KDF-bound — a
   free-tier quota, an expensive report, anything where the attacker's cost
   is not already dominated by the work we make them do. Those limits get
   their value from the number, and the number has to be true.
