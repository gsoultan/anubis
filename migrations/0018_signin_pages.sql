-- ============================================================================
-- 0018_signin_pages.sql — per-tenant sign-in page configuration
--
-- Each tenant brands its own sign-in page. Deliberately a CONSTRAINED token
-- set, not arbitrary markup: the login page is the one page that must never
-- be broken, so the builder offers layout/brand/copy knobs that cannot
-- produce an inaccessible page. The Go tier renders the page from this row;
-- the console's live preview renders the SAME config.
-- ============================================================================
CREATE TABLE signin_pages (
    updated_at timestamptz NOT NULL DEFAULT now(),
    tenant_id  uuid PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    config     jsonb NOT NULL DEFAULT '{}'
);
