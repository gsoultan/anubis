-- ---------------------------------------------------------------------------
-- A sign-in page per REALM, so internal, partner and public populations can
-- have different doors.
--
-- auth_pages could already be bound to an application, or be the tenant
-- default, and could be reached explicitly by slug. What it could not express
-- is "this is the page partners see", which is the distinction tenants
-- actually brand around — an employee portal and a supplier portal look
-- nothing alike even when they are one application.
--
-- The realm IS known when the page renders: internal/auth/adapter/http
-- reads it from ?realm= and falls back to 'internal', which is also what the
-- realm picker posts. So this needs no new plumbing at the edge, only a
-- binding and a step in resolution.
--
-- RESOLUTION IS PURELY ADDITIVE. The existing order is
--     explicit slug -> application -> tenant default
-- and this inserts realm between application and default:
--     explicit slug -> application -> REALM -> tenant default
-- An application that has configured its own page keeps winning, exactly as
-- it does today; nothing that currently resolves changes. Realm pages fill
-- the gap that used to fall straight through to the default.
--
-- The composite foreign key is the same pattern the scope schema uses: it
-- makes a page bound to another tenant's realm physically unrepresentable
-- rather than merely tested for.
-- ---------------------------------------------------------------------------

SET LOCAL lock_timeout = '5s';

ALTER TABLE auth_pages ADD COLUMN realm_id uuid;

ALTER TABLE auth_pages
    ADD CONSTRAINT auth_pages_realm_fkey
    FOREIGN KEY (realm_id, tenant_id)
    REFERENCES realms (id, tenant_id) ON DELETE SET NULL;

-- One page per realm per kind. Partial, so the many pages with no realm
-- binding are unaffected — the same shape as auth_pages_one_per_app.
CREATE UNIQUE INDEX auth_pages_one_per_realm ON auth_pages (realm_id, kind)
    WHERE realm_id IS NOT NULL;

-- A page cannot be bound to both an application and a realm. Resolution would
-- have to pick one, and whichever it picked would surprise somebody; refusing
-- the row is cheaper than documenting a precedence nobody remembers.
ALTER TABLE auth_pages
    ADD CONSTRAINT auth_pages_one_binding
    CHECK (application_id IS NULL OR realm_id IS NULL);

-- The default page must stay reachable: a tenant whose every page is bound
-- has nothing to fall through to.
COMMENT ON COLUMN auth_pages.realm_id IS
    'Optional realm binding. Resolution: slug -> application -> realm -> default.';
