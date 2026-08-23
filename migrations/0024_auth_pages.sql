-- ============================================================================
-- 0024_auth_pages.sql — many sign-in and sign-out pages per tenant
--
-- 0018 allowed exactly one sign-in page per tenant (tenant_id was the primary
-- key). Real tenants need more than one: a staff console and a customer
-- portal look nothing alike, a partner portal carries the partner's brand,
-- and a migration period runs two designs side by side. Sign-out needs the
-- same treatment — the page a user lands on after signing out is part of the
-- product, not an afterthought.
--
-- Each page is addressable by its own URL through `slug`, so a tenant can
-- hand out /p/impack/signin/partners and /p/impack/signin/staff. One page per
-- (tenant, kind) is the default: what /v1/authorize renders when nothing more
-- specific applies.
--
-- A page may bind to an application. That is what makes the URLs meaningful:
-- visiting a bound page starts a real authorization-code flow for that
-- application instead of rendering a form with nowhere to post.
-- ============================================================================

CREATE TABLE auth_pages (
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    application_id uuid,
    is_default     boolean NOT NULL DEFAULT false,
    kind           text NOT NULL CHECK (kind IN ('signin','signout')),
    status         text NOT NULL DEFAULT 'active'
                   CHECK (status IN ('active','disabled')),
    -- The URL segment. Same charset rule as every other slug in this schema.
    slug           text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9_-]{1,62}$'),
    name           text NOT NULL,
    -- A CONSTRAINED token set, never arbitrary markup: the sign-in page is the
    -- one page that must never be broken, and a tenant admin is not a trusted
    -- author of HTML that Anubis serves on its own origin. The application
    -- layer validates every field (internal/tenancy/domain/pagecfg).
    config         jsonb NOT NULL DEFAULT '{}',

    UNIQUE (tenant_id, kind, slug),
    -- A page bound to an application must belong to that application's tenant.
    FOREIGN KEY (application_id, tenant_id)
        REFERENCES applications(id, tenant_id) ON DELETE SET NULL
);

-- Exactly one default per tenant per kind: "which page does /v1/authorize
-- render" must have one answer, decided by the schema rather than by ORDER BY.
CREATE UNIQUE INDEX auth_pages_one_default ON auth_pages (tenant_id, kind)
    WHERE is_default;

-- At most one page per application per kind, so an app-initiated flow never
-- has to choose between two candidates.
CREATE UNIQUE INDEX auth_pages_one_per_app ON auth_pages (application_id, kind)
    WHERE application_id IS NOT NULL;

CREATE INDEX auth_pages_lookup ON auth_pages (tenant_id, kind, status);

-- ---------------------------------------------------------------------------
-- Carry the existing single page over as that tenant's default sign-in page.
-- ---------------------------------------------------------------------------
INSERT INTO auth_pages (tenant_id, kind, slug, name, is_default, config, updated_at)
SELECT tenant_id, 'signin', 'default', 'Default sign-in', true, config, updated_at
  FROM signin_pages
ON CONFLICT DO NOTHING;

-- Every tenant gets a default sign-out page, because signing out with no page
-- configured should still land somewhere branded rather than on a blank 200.
INSERT INTO auth_pages (tenant_id, kind, slug, name, is_default, config)
SELECT t.id, 'signout', 'default', 'Default sign-out', true, '{}'::jsonb
  FROM tenants t
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- Expand/contract, as the rules require: readers of signin_pages keep working
-- against the default page while the application layer moves to auth_pages.
-- A later migration drops the view once nothing reads it.
-- ---------------------------------------------------------------------------
DROP TABLE signin_pages;

CREATE VIEW signin_pages AS
SELECT tenant_id, config, updated_at
  FROM auth_pages
 WHERE kind = 'signin' AND is_default;

-- ---------------------------------------------------------------------------
-- RP-initiated logout needs its own allowlist. Reusing redirect_uris would be
-- wrong in both directions: a login callback is not a place to land after
-- signing out, and an open redirect on the logout endpoint is a phishing
-- primitive ("you have been signed out, sign in again here").
-- Exact match, no wildcards — the same rule as redirect_uris.
-- ---------------------------------------------------------------------------
ALTER TABLE applications
    ADD COLUMN post_logout_redirect_uris text[] NOT NULL DEFAULT '{}';
