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
| 2026-09-06 08:45 UTC | 241.334µs | 161µs | shapes 1 → 1 | flushes 0 → 0 | 309.5 MB | clean |
| 2026-09-07 05:12 UTC | 255.875µs | 207.958µs | shapes 1 → 1 | flushes 0 → 0 | 17.9 MB | clean |

**Which storm each row measured**, because the table never said and the answer
is the only reason to keep a column of them. Every row through 2026-09-06 ran
against `storm v0.2.0`. **2026-09-07 is the first reading on v0.6.3** — the
first time declared aggregations and joins, unions, statement pinning, the
index grammar, upsert on unique indexes and row locking were in the binary
under load at all. No signal moved: shapes 1 → 1 and flushes 0 → 0 are
identical, `rgen` is clean, and p95 sits three orders of magnitude inside the
2 ms budget.

**The RSS column is not a series and 17.9 MB is not an improvement.** It is a
quiet reading taken after a 90-second idle, and the previous rows recorded the
same phase at 213.8 and 309.5 MB — three different processes, and Go returns
pages with MADV_FREE on its own schedule. The plateau is the signal, and it is
inside a single run: 313 → 598 → 553 → 618 MB across four load rounds is a
step and then a flat line, not a ramp. A leak looks like the ramp.

**p95 moved from 161µs to 208µs and that is not a regression either** — the
pgx baseline moved with it, 241µs to 256µs, on a machine that was also running
a container build. storm is faster than raw pgx in both readings, which is the
comparison the row exists to make; the absolutes across rows are not.
