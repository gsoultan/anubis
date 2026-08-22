\set ON_ERROR_STOP on
-- ===========================================================================
-- External identity populations: suppliers, applicants, and the guards that
-- keep them from becoming a privilege-escalation path.
-- ===========================================================================

-- Roles per population. allowed_realm_kinds is the guard.
INSERT INTO roles (tenant_id, name, description, allowed_realm_kinds)
SELECT t.id, v.n, v.d, v.k FROM tenants t, (VALUES
  ('partner.portal',   'Supplier portal user', '{partner}'::text[]),
  ('public.applicant', 'Job applicant',        '{public}'::text[]),
  ('employee.finance', 'Finance staff',        '{internal}'::text[])
) v(n,d,k) WHERE t.slug='impack';

-- A high-assurance permission: approving payment requires a verified identity.
INSERT INTO permissions (application_id, tenant_id, app_slug, resource, action,
                         description, risk, min_assurance)
SELECT a.id, a.tenant_id, a.slug, 'payment', 'approve_high', '', 'critical', 3
  FROM applications a WHERE a.slug='app-1';
-- A low-assurance permission: reading your own application.
INSERT INTO permissions (application_id, tenant_id, app_slug, resource, action,
                         description, min_assurance)
SELECT a.id, a.tenant_id, a.slug, 'application', 'read_own', '', 1
  FROM applications a WHERE a.slug='app-1';
-- Supplier portal permission.
INSERT INTO permissions (application_id, tenant_id, app_slug, resource, action,
                         description, min_assurance)
SELECT a.id, a.tenant_id, a.slug, 'purchase_order', 'read', '', 2
  FROM applications a WHERE a.slug='app-1';

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
 WHERE (r.name='public.applicant'  AND p.key='app-1:application:read_own')
    OR (r.name='partner.portal'    AND p.key='app-1:purchase_order:read')
    OR (r.name='public.applicant'  AND p.key='app-1:payment:approve_high');
INSERT INTO role_permissions_effective (role_id, permission_id, via_role_id)
SELECT role_id, permission_id, role_id FROM role_permissions
 WHERE role_id IN (SELECT id FROM roles WHERE name IN ('public.applicant','partner.portal'))
ON CONFLICT DO NOTHING;

-- Supplier user, scoped to their own company on the partner axis.
INSERT INTO grants (tenant_id, identity_id, role_id, granted_by)
SELECT i.tenant_id, i.id, r.id, i.id
  FROM identities i JOIN realms rl ON rl.id=i.realm_id AND rl.code='partner'
  JOIN roles r ON r.tenant_id=i.tenant_id AND r.name='partner.portal'
 ORDER BY i.id LIMIT 1;
INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id)
SELECT g.id, g.tenant_id, 'partner', n.id
  FROM grants g, LATERAL (SELECT id FROM scope_nodes
                           WHERE node_type='partner_org' AND tenant_id=g.tenant_id
                           ORDER BY id LIMIT 1) n
 WHERE g.role_id=(SELECT id FROM roles WHERE name='partner.portal');

-- Applicant: SELF-SCOPED, no axis constraints.
INSERT INTO grants (tenant_id, identity_id, role_id, granted_by, self_scoped)
SELECT i.tenant_id, i.id, r.id, i.id, true
  FROM identities i JOIN realms rl ON rl.id=i.realm_id AND rl.code='public'
  JOIN roles r ON r.tenant_id=i.tenant_id AND r.name='public.applicant'
 ORDER BY i.id LIMIT 1;

\echo '=== EXTERNAL USER CORRECTNESS ==='
WITH sup AS (
  SELECT g.identity_id, g.tenant_id, gs.scope_node_id AS own_org
    FROM grants g JOIN grant_scopes gs ON gs.grant_id=g.id AND gs.axis_code='partner'
   WHERE g.role_id=(SELECT id FROM roles WHERE name='partner.portal') LIMIT 1),
app AS (
  SELECT g.identity_id, g.tenant_id
    FROM grants g WHERE g.self_scoped
      AND g.role_id=(SELECT id FROM roles WHERE name='public.applicant') LIMIT 1)
SELECT 'supplier reads own company PO -> ALLOW' AS case,
       authorize(identity_id,tenant_id,'app-1:purchase_order:read',
                 jsonb_build_object('partner', own_org)) AS got, true AS want FROM sup
UNION ALL
SELECT 'supplier reads ANOTHER company PO -> DENY',
       authorize(identity_id,tenant_id,'app-1:purchase_order:read',
         jsonb_build_object('partner',(SELECT id FROM scope_nodes
            WHERE node_type='partner_org' AND id<>sup.own_org ORDER BY id LIMIT 1))), false
  FROM sup
UNION ALL
SELECT 'applicant reads OWN record -> ALLOW',
       authorize(identity_id,tenant_id,'app-1:application:read_own',
                 jsonb_build_object('_owner', identity_id)), true FROM app
UNION ALL
SELECT 'applicant reads SOMEONE ELSE record -> DENY',
       authorize(identity_id,tenant_id,'app-1:application:read_own',
                 jsonb_build_object('_owner', gen_random_uuid())), false FROM app
UNION ALL
SELECT 'FAIL-CLOSED: self-scoped, no _owner -> DENY',
       authorize(identity_id,tenant_id,'app-1:application:read_own','{}'::jsonb), false FROM app
UNION ALL
SELECT 'ASSURANCE: IAL1 applicant, IAL3 permission -> DENY',
       authorize(identity_id,tenant_id,'app-1:payment:approve_high',
                 jsonb_build_object('_owner', identity_id)), false FROM app;

\echo ''
\echo '=== DEPROVISIONING (grants left intact on purpose) ==='
UPDATE identities SET status='disabled', disabled_at=now()
 WHERE id=(SELECT identity_id FROM grants WHERE self_scoped LIMIT 1);
WITH app AS (SELECT g.identity_id, g.tenant_id FROM grants g WHERE g.self_scoped LIMIT 1)
SELECT 'disabled identity -> DENY' AS case,
       authorize(identity_id,tenant_id,'app-1:application:read_own',
                 jsonb_build_object('_owner', identity_id)) AS got, false AS want FROM app;
UPDATE identities SET status='active', disabled_at=NULL, anonymized_at=now()
 WHERE id=(SELECT identity_id FROM grants WHERE self_scoped LIMIT 1);
WITH app AS (SELECT g.identity_id, g.tenant_id FROM grants g WHERE g.self_scoped LIMIT 1)
SELECT 'anonymised (retention) identity -> DENY' AS case,
       authorize(identity_id,tenant_id,'app-1:application:read_own',
                 jsonb_build_object('_owner', identity_id)) AS got, false AS want FROM app;
UPDATE identities SET anonymized_at=NULL WHERE anonymized_at IS NOT NULL;

\echo ''
\echo '=== REALM ISOLATION ==='
SELECT 'same username across 3 realms' AS case, count(*) AS got, 3 AS want
  FROM identities WHERE lower(username)='user1';

\echo ''
\echo '=== ESCALATION GUARDS (must all be rejected) ==='
\set ON_ERROR_STOP off
\echo '--- employee-only role granted to a public account ---'
INSERT INTO grants (tenant_id, identity_id, role_id, granted_by)
SELECT i.tenant_id, i.id, r.id, i.id
  FROM identities i JOIN realms rl ON rl.id=i.realm_id AND rl.code='public'
  JOIN roles r ON r.tenant_id=i.tenant_id AND r.name='employee.finance'
 ORDER BY i.id LIMIT 1;

\echo '--- identity carrying a category from ANOTHER realm ---'
UPDATE identities SET category_id =
  (SELECT rc.id FROM realm_categories rc JOIN realms r ON r.id=rc.realm_id
    WHERE r.code='public' LIMIT 1)
 WHERE id = (SELECT i.id FROM identities i JOIN realms r ON r.id=i.realm_id
              AND r.code='partner' LIMIT 1);

\echo '--- axis constraint attached to a self-scoped grant ---'
INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id)
SELECT g.id, g.tenant_id, 'org', n.id
  FROM grants g, LATERAL (SELECT id FROM scope_nodes WHERE node_type='department'
                           AND tenant_id=g.tenant_id ORDER BY id LIMIT 1) n
 WHERE g.self_scoped LIMIT 1;
