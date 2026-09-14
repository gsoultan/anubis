-- ---------------------------------------------------------------------------
-- Where an application's catalog COMES FROM, and what happened last time it
-- arrived.
--
-- ApplyManifest already accepts a permission/role/route document over the
-- API, and since the catalog document gained a CSV form an operator can
-- upload one by hand. Neither answers the case every directory eventually
-- has: the catalog is maintained somewhere else — an ERP export, a file a
-- platform team publishes, a spreadsheet in a share — and Anubis should go
-- and read it rather than wait for somebody to remember.
--
-- A source is PINNED TO ONE APPLICATION by foreign key. That pin is the
-- security story: a source names a URL Anubis will fetch and then apply to a
-- catalog, which is a privilege-escalation channel if the document can
-- choose its own scope. It cannot — permissions are upserted under the
-- application's id and slug, so the very worst a compromised feed achieves
-- is a mess inside the one application it was configured for.
--
-- WHY A RUN CARRIES THE DOCUMENT'S DIGEST. Applying a manifest bumps
-- applications.manifest_version, and the version is what tells the gate its
-- snapshot is stale (0040). A source polled every five minutes with an
-- unchanged file would therefore rebuild a gate snapshot 288 times a day to
-- install exactly the same rows. An identical digest is recorded as
-- 'skipped' and nothing is written — the same reasoning as 0040 itself, one
-- layer further out.
--
-- The floor on interval_seconds is not tidiness. A catalog is a document a
-- human edits a few times a quarter; anything faster than five minutes is
-- Anubis hammering somebody else's server for a file that did not change.
-- ---------------------------------------------------------------------------

SET LOCAL lock_timeout = '5s';

CREATE TABLE catalog_sync_sources (
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    last_run_at      timestamptz,
    -- NULL is a source that only ever runs when somebody asks: a manual
    -- source and a scheduled one differ by this column and nothing else.
    next_run_at      timestamptz,
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    application_id   uuid NOT NULL,
    kind             text NOT NULL CHECK (kind IN ('http')),
    format           text NOT NULL CHECK (format IN ('json','csv')),
    status           text NOT NULL DEFAULT 'active'
                     CHECK (status IN ('active','disabled')),
    name             text NOT NULL,
    -- http: {"url": "https://...", "auth_header": "Bearer ..."}
    config           jsonb NOT NULL,
    interval_seconds integer NOT NULL DEFAULT 0
                     CHECK (interval_seconds = 0 OR interval_seconds >= 300),

    -- The pin. A composite FK, like the scope schema uses: a source pointed
    -- at another tenant's application is unrepresentable, not merely tested.
    FOREIGN KEY (application_id, tenant_id)
        REFERENCES applications(id, tenant_id) ON DELETE CASCADE,
    UNIQUE (tenant_id, name)
);

-- The scheduler's only query: due, active, in interval order. A partial
-- index because the manual sources (next_run_at IS NULL) are never due and
-- have no business being scanned.
CREATE INDEX catalog_sync_sources_due
    ON catalog_sync_sources (next_run_at)
    WHERE status = 'active' AND next_run_at IS NOT NULL;

CREATE TABLE catalog_sync_runs (
    started_at   timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz,
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    source_id    uuid NOT NULL REFERENCES catalog_sync_sources(id) ON DELETE CASCADE,
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    dry          boolean NOT NULL DEFAULT false,
    status       text NOT NULL DEFAULT 'running'
                 CHECK (status IN ('running','ok','failed','dry_run','skipped')),
    -- 'system' for a scheduled run, the operator's id when somebody pressed
    -- the button. A run history that cannot say which is which cannot answer
    -- the only question asked after a surprise: who did this.
    actor        text NOT NULL DEFAULT 'system',
    -- sha256 of the fetched document, so an unchanged catalog is skipped
    -- rather than reapplied.
    document_sha text NOT NULL DEFAULT '',
    -- the apply report, or the reason there was not one
    report       jsonb,
    error        text NOT NULL DEFAULT ''
);

CREATE INDEX catalog_sync_runs_by_source
    ON catalog_sync_runs (source_id, started_at DESC);
