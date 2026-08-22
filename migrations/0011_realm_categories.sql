-- ============================================================================
-- 0011_realm_categories.sql — runtime-extensible identity categories per realm
--
-- "Supplier" vs "contractor" inside the partner realm, "applicant" vs
-- "customer" inside the public realm — and whatever a tenant invents later.
-- Adding one is an INSERT, never a deploy: the same rule as scope axes.
--
-- DELIBERATE LIMIT: a category is DIRECTORY CLASSIFICATION, not an
-- authorization input. authorize() never reads it. If two categories need
-- different sign-in policy, that is a second realm of the same kind (realms
-- are rows; several per kind are legal). If they need different access, that
-- is roles. Letting categories decide access would recreate the jsonb-decides
-- trap ADR-0003 closed.
-- ============================================================================

CREATE TABLE realm_categories (
    created_at   timestamptz NOT NULL DEFAULT now(),
    sort_order   int NOT NULL DEFAULT 100,
    id           uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id    uuid NOT NULL,
    realm_id     uuid NOT NULL,
    code         text NOT NULL CHECK (code ~ '^[a-z][a-z0-9_]{1,30}$'),
    display_name text NOT NULL,

    UNIQUE (realm_id, code),
    -- composite-FK target so identities can prove same-realm membership
    UNIQUE (id, realm_id),
    FOREIGN KEY (realm_id, tenant_id) REFERENCES realms(id, tenant_id) ON DELETE CASCADE
);

-- The invariant: an identity cannot carry a category from a DIFFERENT realm.
-- Enforced by the schema, not by form validation — same pattern as
-- grant_scopes' cross-axis guard.
ALTER TABLE identities ADD COLUMN category_id uuid;
ALTER TABLE identities
    ADD CONSTRAINT identities_category_same_realm
    FOREIGN KEY (category_id, realm_id)
    REFERENCES realm_categories (id, realm_id) ON DELETE SET NULL;

CREATE INDEX identities_category ON identities (category_id)
    WHERE category_id IS NOT NULL;
