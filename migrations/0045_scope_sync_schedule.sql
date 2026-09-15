-- ============================================================================
-- 0045_scope_sync_schedule.sql — structures that refresh themselves
--
-- 0017 gave a structure a source of truth and a reconciler; what it never gave
-- it was a clock. Every sync ran because an operator pressed a button, which
-- means an org chart is only as current as the last person who remembered it.
-- The four channels a catalog can arrive through (0043) already included a
-- scheduled one; this is the same column pair on the scope side, deliberately
-- identical so the two schedulers cannot drift into two behaviours.
--
--   next_run_at NULL      a source that only ever runs when somebody asks.
--                         A manual source and a scheduled one differ by this
--                         column and nothing else — nothing existing changes
--                         behaviour, because every row starts NULL.
--   interval_seconds 0    the same statement, said by the operator.
--
-- The floor on interval_seconds is not tidiness. A structure is an org chart
-- or a customer list: it changes when HR or sales changes it, a few times a
-- week at most. Anything faster than five minutes is Anubis hammering
-- somebody else's ERP for rows that did not move.
--
-- The zero-row refusal in the app tier matters more here than it does for a
-- catalog. A catalog feed returning nothing applies nothing; a SCOPE feed
-- returning nothing means "every node you have is gone", and the reconciler
-- would dutifully archive the whole axis. RunSync already refuses an empty
-- feed, and a scheduled run goes through that same path — this migration adds
-- no way around it.
-- ============================================================================

SET LOCAL lock_timeout = '5s';

ALTER TABLE scope_sync_sources
    -- NULL is a source that only ever runs when somebody asks.
    ADD COLUMN next_run_at      timestamptz,
    ADD COLUMN interval_seconds integer NOT NULL DEFAULT 0
                                CHECK (interval_seconds = 0 OR interval_seconds >= 300);

-- The scheduler's only query: due, active, in due order. A partial index
-- because the manual sources (next_run_at IS NULL) are never due and have no
-- business being scanned — a tick that finds nothing must cost one index
-- probe, not a scan of every source in the installation.
CREATE INDEX scope_sync_sources_due
    ON scope_sync_sources (next_run_at)
    WHERE status = 'active' AND next_run_at IS NOT NULL;

COMMENT ON COLUMN scope_sync_sources.next_run_at IS
    'When the scheduler should next run this source. NULL = manual only.';
COMMENT ON COLUMN scope_sync_sources.interval_seconds IS
    'Seconds between scheduled runs. 0 = manual only. Floor of 300 when set.';
