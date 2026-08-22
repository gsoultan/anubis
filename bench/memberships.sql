\set ON_ERROR_STOP on
-- Membership semantics: assign fans out, unassign revokes, guards still bite,
-- multiple memberships union.
CREATE TEMP TABLE ms AS
WITH t AS (SELECT id FROM tenants WHERE slug='impack'),
r AS (SELECT id AS realm_id, tenant_id FROM realms WHERE code='internal' LIMIT 1),
i AS (INSERT INTO identities (tenant_id, realm_id, assurance_level, username, email)
      SELECT tenant_id, realm_id, 3, 'member_probe', 'mp@test.local' FROM r
      RETURNING id, tenant_id),
role1 AS (SELECT r.id, p.key FROM roles r
           JOIN role_permissions_effective rpe ON rpe.role_id=r.id
           JOIN permissions p ON p.id=rpe.permission_id
          WHERE r.tenant_id=(SELECT id FROM t) ORDER BY r.id, p.key LIMIT 1),
office AS (SELECT id FROM scope_nodes WHERE node_type='office'
            AND tenant_id=(SELECT id FROM t) ORDER BY id LIMIT 1),
m AS (INSERT INTO memberships (tenant_id, name)
      SELECT id, 'Jakarta Finance Team' FROM t RETURNING id, tenant_id),
e AS (INSERT INTO membership_entries (membership_id, tenant_id, role_id)
      SELECT m.id, m.tenant_id, role1.id FROM m, role1 RETURNING id, membership_id, tenant_id),
es AS (INSERT INTO membership_entry_scopes (entry_id, tenant_id, axis_code, scope_node_id)
       SELECT e.id, e.tenant_id, 'org', office.id FROM e, office RETURNING entry_id)
SELECT i.id AS identity_id, i.tenant_id, (SELECT id FROM m) AS membership_id,
       (SELECT key FROM role1) AS perm,
       (SELECT c.descendant_id FROM scope_closure c
          JOIN scope_nodes n ON n.id=c.descendant_id AND n.node_type='team'
         WHERE c.ancestor_id=(SELECT id FROM office) LIMIT 1) AS team
FROM i;

SELECT '  assign fan-out: '||membership_assign(identity_id, membership_id, identity_id)||' grant(s)' FROM ms;

SELECT '  member sees team via membership -> '||
  authorize(identity_id, tenant_id, perm, jsonb_build_object('org', team))||' (want t)' FROM ms;

SELECT '  unassign revoked: '||membership_unassign(identity_id, membership_id)||' grant(s)' FROM ms;
SELECT '  after unassign -> '||
  authorize(identity_id, tenant_id, perm, jsonb_build_object('org', team))||' (want f)' FROM ms;

-- guard: internal-only membership on a PUBLIC applicant must abort
\set ON_ERROR_STOP off
SELECT membership_assign(
  (SELECT i.id FROM identities i JOIN realms r ON r.id=i.realm_id
    WHERE r.code='public' LIMIT 1),
  (SELECT membership_id FROM ms),
  (SELECT identity_id FROM ms));
\set ON_ERROR_STOP on

DELETE FROM identities WHERE username='member_probe';
DELETE FROM memberships WHERE name='Jakarta Finance Team';
