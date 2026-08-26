# raorm soak readings

The two-week soak gating raorm's v0.1.0 tag (2026-08-25 → 2026-09-08). Record
one row per reading with `scripts/soak-record.sh`.

What would move the tag, from raorm's `docs/PRODUCTION-READINESS.md` P3: an
authorize p95 past the 2 ms budget, a shape count that grows with traffic
instead of plateauing, or resident memory without a plateau. The date is not
the gate; these are.

| when | p95 via pgx | p95 via raorm | shapes | flushes | anubisd RSS | rgen |
|---|---|---|---|---|---|---|
| 2026-08-26 04:01 UTC | 229.041µs | 195.166µs | shapes 1 → 1 | flushes 0 → 0 | not running | clean |
