-- ============================================================================
-- 0001_foundation.sql — extensions, conventions, tenancy, catalog versioning
--
-- CONVENTIONS USED THROUGHOUT (performance-relevant, read before editing):
--
--  1. PRIMARY KEYS ARE uuidv7(), NEVER uuidv4()/gen_random_uuid().
--     uuidv7 is time-ordered => index inserts land on the rightmost B-tree
--     page instead of scattering. This eliminates random page splits, keeps
--     the hot leaf pages in shared_buffers, and dramatically reduces WAL
--     full-page-image traffic. On insert-heavy tables (audit_log,
--     refresh_tokens) this is the single biggest win available.
--
--  2. COLUMN ORDER IS 8-BYTE -> 4-BYTE -> 1-BYTE/varlena.
--     Postgres pads columns to their alignment boundary. Declaring
--     timestamptz(8) before int(4) before bool(1) before text removes dead
--     padding bytes. On wide, high-row-count tables this is 10-20% fewer
--     pages, which is 10-20% less I/O and more rows resident in cache.
--
--  3. HOT-UPDATE TABLES GET fillfactor<100.
--     Leaves free space on each page so UPDATEs can write the new tuple
--     version on the SAME page (HOT update) and skip updating every index.
--
--  4. PARTIAL INDEXES OVER FULL ONES wherever a predicate is always present
--     (e.g. WHERE revoked_at IS NULL). Smaller index = fewer levels = fewer
--     buffer reads per probe.
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- digest() for token hashing

-- Deliberately NOT using ltree: see 0003. Closure table needs no extension.

-- ---------------------------------------------------------------------------
-- Migration bookkeeping
-- ---------------------------------------------------------------------------
CREATE TABLE schema_migrations (
    applied_at timestamptz NOT NULL DEFAULT now(),
    version    text PRIMARY KEY,
    checksum   text NOT NULL
);

-- ---------------------------------------------------------------------------
-- Tenants
-- ---------------------------------------------------------------------------
CREATE TABLE tenants (
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    status     text NOT NULL DEFAULT 'active'
               CHECK (status IN ('active','suspended','archived')),
    slug       text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9_-]{1,62}$'),
    name       text NOT NULL,
    settings   jsonb NOT NULL DEFAULT '{}'
) WITH (fillfactor = 90);

-- ---------------------------------------------------------------------------
-- Applications (relying parties). Own their permission + route catalogs.
-- ---------------------------------------------------------------------------
CREATE TABLE applications (
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    access_token_ttl     interval NOT NULL DEFAULT '10 minutes',
    refresh_token_ttl    interval NOT NULL DEFAULT '30 days',
    id                   uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id            uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    manifest_version     int  NOT NULL DEFAULT 0,
    kind                 text NOT NULL
                         CHECK (kind IN ('spa','native','server','service')),
    status               text NOT NULL DEFAULT 'active'
                         CHECK (status IN ('active','disabled')),
    slug                 text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9_-]{1,62}$'),
    name                 text NOT NULL,
    client_secret_hash   text,
    -- EXACT-MATCH allowlist. No wildcards, ever: open redirect = account takeover.
    redirect_uris        text[] NOT NULL DEFAULT '{}',
    backchannel_logout_uri text,
    token_format         text NOT NULL DEFAULT 'v4.public'
                         CHECK (token_format IN ('v4.public','jws.eddsa')),

    UNIQUE (tenant_id, slug),
    -- composite-FK targets: let children prove they share a tenant, and let
    -- permissions carry a provably-correct copy of the slug (see 0004).
    UNIQUE (id, tenant_id),
    UNIQUE (id, slug)
) WITH (fillfactor = 90);

-- ---------------------------------------------------------------------------
-- catalog_version — the single invalidation signal.
--
-- Every table that feeds an in-memory authorization snapshot bumps this.
-- Readers compare their loaded version; a mismatch triggers a reload.
-- One counter beats per-table timestamps: it is a single indexed read, and
-- it cannot skew between tables.
-- ---------------------------------------------------------------------------
CREATE TABLE catalog_version (
    changed_at timestamptz NOT NULL DEFAULT now(),
    version    bigint NOT NULL DEFAULT 1,
    tenant_id  uuid PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE
) WITH (fillfactor = 70);   -- updated constantly, keep HOT updates on-page

CREATE OR REPLACE FUNCTION bump_catalog_version(p_tenant uuid)
RETURNS void LANGUAGE plpgsql AS $fn$
BEGIN
    INSERT INTO catalog_version (tenant_id, version, changed_at)
         VALUES (p_tenant, 1, now())
    ON CONFLICT (tenant_id) DO UPDATE
            SET version = catalog_version.version + 1,
                changed_at = now();
    -- push invalidation; pollers act as the backstop when NOTIFY is dropped
    PERFORM pg_notify('anubis_catalog', p_tenant::text);
END;
$fn$;

-- Generic trigger: any table with a tenant_id column can attach this.
CREATE OR REPLACE FUNCTION trg_bump_catalog()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE
    v_tenant uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        EXECUTE format('SELECT ($1).%I', TG_ARGV[0]) INTO v_tenant USING OLD;
    ELSE
        EXECUTE format('SELECT ($1).%I', TG_ARGV[0]) INTO v_tenant USING NEW;
    END IF;
    IF v_tenant IS NOT NULL THEN
        PERFORM bump_catalog_version(v_tenant);
    END IF;
    RETURN NULL;   -- AFTER trigger
END;
$fn$;
