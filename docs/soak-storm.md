# storm soak readings

The two-week soak gating storm's v0.1.0 tag (2026-08-25 → 2026-09-08). Record
one row per reading with `scripts/soak-record.sh`.

What would move the tag, from storm's `docs/PRODUCTION-READINESS.md` P3: an
authorize p95 past the 2 ms budget, a shape count that grows with traffic
instead of plateauing, or resident memory without a plateau. The date is not
the gate; these are.

| when | p95 via pgx | p95 via storm | shapes | flushes | anubisd RSS | rgen |
|---|---|---|---|---|---|---|
| 2026-09-01 03:20 UTC | 296.917µs | 181.041µs | shapes 1 → 1 | flushes 0 → 0 | 213.8 MB | clean |
