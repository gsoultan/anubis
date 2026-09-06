-- ---------------------------------------------------------------------------
-- Every tenant gets a default sign-in and sign-out page.
--
-- Migration 0024 created auth_pages and seeded a default pair for the tenants
-- that existed WHEN IT RAN. Nothing seeded them afterwards, so every tenant
-- created since — by `anubisd bootstrap`, by the setup installer, or through
-- CreateTenant — has had none at all.
--
-- Sign-in still worked: resolvePage falls through to pagecfg defaults when it
-- finds no row, so /v1/authorize renders. What did not work was everything
-- that expects the row to exist. /p/{tenant}/signin/default answered 404 —
-- including for the URL the console itself displays for a page — and the page
-- builder opened on an empty list, so a new tenant appeared to have no pages
-- rather than two.
--
-- This is a trigger rather than two more lines in bootstrap because there are
-- already two paths that create a tenant and nothing stops a third. Seeding
-- where the row is written means no caller has to remember.
--
-- Statement level with a transition table, like migrations 0006 and 0040.
-- ---------------------------------------------------------------------------

SET LOCAL lock_timeout = '5s';

CREATE OR REPLACE FUNCTION trg_seed_auth_pages()
RETURNS trigger LANGUAGE plpgsql AS $fn$
BEGIN
    INSERT INTO auth_pages (tenant_id, kind, slug, name, is_default, config)
    SELECT n.id, k.kind, 'default', k.label, true, '{}'::jsonb
      FROM newtab n
      CROSS JOIN (VALUES ('signin',  'Default sign-in'),
                         ('signout', 'Default sign-out')) AS k(kind, label)
    ON CONFLICT DO NOTHING;
    RETURN NULL;
END;
$fn$;

CREATE TRIGGER seed_auth_pages
    AFTER INSERT ON tenants
    REFERENCING NEW TABLE AS newtab
    FOR EACH STATEMENT EXECUTE FUNCTION trg_seed_auth_pages();

-- Backfill: every tenant created between 0024 and now is missing its pair.
-- An empty config is exactly what 0024 inserted for sign-out, and pagecfg
-- fills the defaults on read, so these render identically to a page that was
-- never touched.
INSERT INTO auth_pages (tenant_id, kind, slug, name, is_default, config)
SELECT t.id, k.kind, 'default', k.label, true, '{}'::jsonb
  FROM tenants t
  CROSS JOIN (VALUES ('signin',  'Default sign-in'),
                     ('signout', 'Default sign-out')) AS k(kind, label)
 WHERE NOT EXISTS (
     SELECT 1 FROM auth_pages p WHERE p.tenant_id = t.id AND p.kind = k.kind
 )
ON CONFLICT DO NOTHING;
