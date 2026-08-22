-- ============================================================================
-- 0012_internal_kind.sql — rename realm kind 'employee' → 'internal'
--
-- Same lesson as renaming the realm display names, one level down: the KIND is
-- the population's relationship to the tenant, and 'employee' over-specified
-- it. Internal people are employees, interns, board members, seconded staff —
-- which is exactly what realm_categories (0011) is for. Generic kind,
-- specific categories.
--
-- Forward-only: 0008 created the CHECK with 'employee'; this migration
-- rewrites constraint + data + default in one transaction.
-- ============================================================================

ALTER TABLE realms DROP CONSTRAINT realms_kind_check;
UPDATE realms SET kind = 'internal' WHERE kind = 'employee';
ALTER TABLE realms ADD CONSTRAINT realms_kind_check
    CHECK (kind IN ('internal','partner','public','service'));

-- Every role that named the old kind follows automatically.
UPDATE roles
   SET allowed_realm_kinds = array_replace(allowed_realm_kinds, 'employee', 'internal')
 WHERE 'employee' = ANY (allowed_realm_kinds);
ALTER TABLE roles ALTER COLUMN allowed_realm_kinds SET DEFAULT '{internal}';

-- The default internal realm follows the same generic-name rule, and gets its
-- first category: 'Employee' is now a CATEGORY inside Internal, not the kind.
UPDATE realms SET code = 'internal', display_name = 'Internal' WHERE code = 'employee';

INSERT INTO realm_categories (tenant_id, realm_id, code, display_name, sort_order)
SELECT r.tenant_id, r.id, 'employee', 'Employee', 10
  FROM realms r WHERE r.code = 'internal'
ON CONFLICT (realm_id, code) DO NOTHING;
