-- ============================================================================
-- 0028 — Anubis's own applications are not the tenant's applications.
--
-- An application is a RELYING PARTY: something a tenant's people sign in to.
-- Two rows in every tenant are not that. `anubis` exists to own Anubis's own
-- permission catalog, and `console` was the OIDC client the admin console used
-- back when it signed in as a tenant identity — which it no longer does, since
-- operators are a separate population entirely (ADR-0011).
--
-- Listing them alongside a tenant's real applications invites somebody to
-- edit the redirect URIs of the thing they are editing them with, or to add a
-- permission to Anubis's own catalog by hand. They stay in the table because
-- the permission catalog hangs off `anubis`; they come out of the list.
-- ============================================================================

ALTER TABLE applications
    ADD COLUMN is_system boolean NOT NULL DEFAULT false;

-- Existing installations: the two Anubis provisions itself.
UPDATE applications SET is_system = true WHERE slug IN ('anubis', 'console');

-- The tenant-facing list filters on this, so an index earns its keep.
CREATE INDEX applications_tenant_visible
    ON applications (tenant_id) WHERE NOT is_system;

COMMENT ON COLUMN applications.is_system IS
    'Anubis''s own applications (ADR-0011). Present so the permission catalog '
    'has an owner; hidden from the tenant''s application list.';
