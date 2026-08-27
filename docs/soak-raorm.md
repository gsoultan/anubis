# storm soak readings

The two-week soak gating storm's v0.1.0 tag (2026-08-25 → 2026-09-08). Record
one row per reading with `scripts/soak-record.sh`.

What would move the tag, from storm's `docs/PRODUCTION-READINESS.md` P3: an
authorize p95 past the 2 ms budget, a shape count that grows with traffic
instead of plateauing, or resident memory without a plateau. The date is not
the gate; these are.

**Rows with "not running" measured a test loop, not a server** — the harness
could not see a live process until `scripts/soak-load.sh` existed (2026-08-26).
Two of the four signals, resident memory and shape growth under traffic, had
no source before that.

First run under real traffic, ~12,200 authorize decisions/s through the whole
stack, four rounds:

| phase | RSS MB | heapInuse | heapAlloc | GCs |
|---|---|---|---|---|
| idle | 265 | 235 | 208 | 16 |
| after round 1 | 687 | 274 | 212 | 24 |
| after round 2 | 756 | 272 | 190 | 32 |
| after round 3 | 766 | 353 | 272 | 39 |
| after round 4 | 766 | 271 | 194 | 48 |

**Live heap is flat across the load rounds while RSS triples.** Nothing
accumulates per request — which is the claim storm makes about its request
path — and the RSS climb is pages Go has not returned rather than data it is
holding. Shapes stayed at 1 with 0 flushes throughout.

Read these as comparisons, not absolutes: on macOS RSS can sit *below*
heapInuse (MADV_FREE), and heapInuse counts spans including the free objects
inside them. `soak-load.sh` prints that caveat with every run.

| when | p95 via pgx | p95 via storm | shapes | flushes | anubisd RSS | rgen |
|---|---|---|---|---|---|---|
| 2026-08-26 04:01 UTC | 229.041µs | 195.166µs | shapes 1 → 1 | flushes 0 → 0 | not running | clean |
| 2026-08-26 11:27 UTC | 226.959µs | 200.417µs | shapes 1 → 1 | flushes 0 → 0 | not running | clean |
| 2026-08-26 12:08 UTC | 232.917µs | 202.667µs | shapes 1 → 1 | flushes 0 → 0 | 668.2 MB | clean |
| 2026-08-26 12:11 UTC | 777.459µs | 198µs | shapes 1 → 1 | flushes 0 → 0 | 611.3 MB | clean |
| 2026-08-26 12:15 UTC | 222.417µs | 194.25µs | shapes 1 → 1 | flushes 0 → 0 | 667.0 MB | clean |
| 2026-08-27 01:06 UTC | 234.5µs | 253.333µs | shapes 1 → 1 | flushes 0 → 0 | 22.8 MB | clean |
